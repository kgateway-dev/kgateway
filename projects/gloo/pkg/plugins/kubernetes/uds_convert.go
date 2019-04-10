package kubernetes

import (
	"context"
	"crypto/md5"
	"fmt"
	"reflect"
	"sort"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var ignoredLabels = []string{
	"pod-template-hash",        // it is common and provides nothing useful for discovery
	"controller-revision-hash", // set by helm
	"pod-template-generation",  // set by helm
	"release",                  // set by helm
}

type UpstreamConverter interface {
	UpstreamsForService(ctx context.Context, svc *kubev1.Service, pods []*kubev1.Pod) v1.UpstreamList
}

func DefaultUpstreamConverter() *KubeUpstreamConverter {
	kuc := new(KubeUpstreamConverter)
	kuc.createUpstream = createUpstream
	return kuc
}

type KubeUpstreamConverter struct {
	createUpstream func(ctx context.Context, svc *kubev1.Service, port kubev1.ServicePort, labels map[string]string) *v1.Upstream
}

func (uc *KubeUpstreamConverter) UpstreamsForService(ctx context.Context, svc *kubev1.Service, pods []*kubev1.Pod) v1.UpstreamList {

	uniqueLabelSets := GetUniqueLabelSets(svc, pods)
	return uc.CreateUpstreamForLabels(ctx, uniqueLabelSets, svc)
}

func (uc *KubeUpstreamConverter) CreateUpstreamForLabels(ctx context.Context, uniqueLabelSets []map[string]string, svc *kubev1.Service) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for _, extendedLabels := range uniqueLabelSets {
		for _, port := range svc.Spec.Ports {
			upstreams = append(upstreams, uc.createUpstream(ctx, svc, port, extendedLabels))
		}
	}
	return upstreams
}

func GetUniqueLabelSets(svc *kubev1.Service, pods []*kubev1.Pod) []map[string]string {

	var podlabelss []map[string]string
	for _, pod := range pods {
		if pod.Namespace != svc.Namespace {
			continue
		}
		podlabelss = append(podlabelss, pod.ObjectMeta.Labels)
	}

	return GetUniqueLabelSetsForObjects(svc.Spec.Selector, podlabelss)
}

func GetUniqueLabelSetsForObjects(selector map[string]string, podlabelss []map[string]string) []map[string]string {
	uniqueLabelSets := []map[string]string{
		selector,
	}
	if len(selector) > 0 {
		for _, podlabels := range podlabelss {
			if !labels.AreLabelsInWhiteList(selector, podlabels) {
				continue
			}

			// create upstreams for the extra labels beyond the selector
			extendedLabels := make(map[string]string)
		addExtendedLabels:
			for k, v := range podlabels {
				// special cases we ignore
				for _, ignoredLabel := range ignoredLabels {
					if k == ignoredLabel {
						continue addExtendedLabels
					}
				}
				extendedLabels[k] = v
			}
			if len(extendedLabels) > 0 && !containsMap(uniqueLabelSets, extendedLabels) {
				uniqueLabelSets = append(uniqueLabelSets, extendedLabels)
			}
		}

	}
	return uniqueLabelSets
}

func createUpstream(ctx context.Context, svc *kubev1.Service, port kubev1.ServicePort, labels map[string]string) *v1.Upstream {
	meta := svc.ObjectMeta
	coremeta := kubeutils.FromKubeMeta(meta)
	coremeta.ResourceVersion = ""
	extraLabels := make(map[string]string)
	// find extra keys not present in the service selector
	for k, v := range labels {
		if _, ok := svc.Spec.Selector[k]; ok {
			continue
		}
		extraLabels[k] = v
	}
	coremeta.Name = strings.ToLower(UpstreamName(meta.Namespace, meta.Name, port.Port, extraLabels))
	return &v1.Upstream{
		Metadata: coremeta,
		UpstreamSpec: &v1.UpstreamSpec{
			UpstreamType: &v1.UpstreamSpec_Kube{
				Kube: &kubeplugin.UpstreamSpec{
					ServiceName:      meta.Name,
					ServiceNamespace: meta.Namespace,
					ServicePort:      uint32(port.Port),
					Selector:         labels,
				},
			},
		},
		DiscoveryMetadata: &v1.DiscoveryMetadata{},
	}
}

func UpstreamName(serviceNamespace, serviceName string, servicePort int32, extraLabels map[string]string) string {
	const maxLen = 63

	var labelsTag string
	if len(extraLabels) > 0 {
		_, values := keysAndValues(extraLabels)
		labelsTag = fmt.Sprintf("-%v", strings.Join(values, "-"))
	}
	name := fmt.Sprintf("%s-%s%s-%v", serviceNamespace, serviceName, labelsTag, servicePort)
	if len(name) > maxLen {
		hash := md5.Sum([]byte(name))
		hexhash := fmt.Sprintf("%x", hash)
		name = name[:maxLen-len(hexhash)] + hexhash
	}
	name = strings.Replace(name, ".", "-", -1)
	return name
}

// TODO: move to a utils package

func containsString(s string, slice []string) bool {
	for _, s2 := range slice {
		if s2 == s {
			return true
		}
	}
	return false
}

func containsMap(maps []map[string]string, item map[string]string) bool {
	for _, m := range maps {
		if reflect.DeepEqual(m, item) {
			return true
		}
	}
	return false
}

func keysAndValues(m map[string]string) ([]string, []string) {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var values []string
	for _, k := range keys {
		values = append(values, m[k])
	}
	return keys, values
}

func skip(svc *kubev1.Service, opts discovery.Opts) bool {
	// ilackarms: allow user to override the skip with an annotation
	// force discovery for a service with no selector
	if svc.ObjectMeta.Annotations[discoveryAnnotationKey] == discoveryAnnotationTrue {
		return false
	}
	// note: ilackarms: IgnoredServices is not set anywhere
	for _, name := range opts.KubeOpts.IgnoredServices {
		if svc.Name == name {
			return true
		}
	}
	return false
}

func (p *plugin) UpdateUpstream(original, desired *v1.Upstream) (bool, error) {
	return UpdateUpstream(original, desired)
}

func UpdateUpstream(original, desired *v1.Upstream) (bool, error) {
	originalSpec, ok := original.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Kube)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.UpstreamSpec_Kube, got %v", reflect.TypeOf(original.UpstreamSpec.UpstreamType).Name())
	}
	desiredSpec, ok := desired.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Kube)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.UpstreamSpec_Kube, got %v", reflect.TypeOf(original.UpstreamSpec.UpstreamType).Name())
	}
	// copy service spec, we don't want to overwrite that
	desiredSpec.Kube.ServiceSpec = originalSpec.Kube.ServiceSpec
	// copy labels; user may have written them over. cannot be auto-discovered
	desiredSpec.Kube.Selector = originalSpec.Kube.Selector

	// do not override ssl and subset config if none specified by discovery
	if desired.UpstreamSpec.SslConfig == nil {
		desired.UpstreamSpec.SslConfig = original.UpstreamSpec.SslConfig
	}
	if desired.UpstreamSpec.CircuitBreakers == nil {
		desired.UpstreamSpec.CircuitBreakers = original.UpstreamSpec.CircuitBreakers
	}
	if desired.UpstreamSpec.LoadBalancerConfig == nil {
		desired.UpstreamSpec.LoadBalancerConfig = original.UpstreamSpec.LoadBalancerConfig
	}
	if desired.UpstreamSpec.ConnectionConfig == nil {
		desired.UpstreamSpec.ConnectionConfig = original.UpstreamSpec.ConnectionConfig
	}

	if desiredSubsetMutator, ok := desired.UpstreamSpec.UpstreamType.(v1.SubsetSpecMutator); ok {
		if desiredSubsetMutator.GetSubsetSpec() == nil {
			desiredSubsetMutator.SetSubsetSpec(original.UpstreamSpec.UpstreamType.(v1.SubsetSpecGetter).GetSubsetSpec())
		}
	}

	if originalSpec.Equal(desiredSpec) {
		return false, nil
	}

	return true, nil
}
