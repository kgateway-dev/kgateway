package kubernetes

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes/serviceconverter"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"

	sanitizer "github.com/solo-io/go-utils/kubeutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
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
	kuc.serviceConverters = serviceconverter.DefaultServiceConverters
	return kuc
}

type KubeUpstreamConverter struct {
	serviceConverters []serviceconverter.ServiceConverter
}

func (uc *KubeUpstreamConverter) UpstreamsForService(ctx context.Context, svc *kubev1.Service, pods []*kubev1.Pod) v1.UpstreamList {

	uniqueLabelSets := GetUniqueLabelSets(svc, pods)
	return uc.createUpstreamForLabels(ctx, uniqueLabelSets, svc)
}

func (uc *KubeUpstreamConverter) createUpstreamForLabels(ctx context.Context, uniqueLabelSets []map[string]string, svc *kubev1.Service) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for _, extendedLabels := range uniqueLabelSets {
		for _, port := range svc.Spec.Ports {
			upstreams = append(upstreams, uc.CreateUpstream(ctx, svc, port, extendedLabels))
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

func (uc *KubeUpstreamConverter) CreateUpstream(ctx context.Context, svc *kubev1.Service, port kubev1.ServicePort, labels map[string]string) *v1.Upstream {
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

	us := &v1.Upstream{
		Metadata: coremeta,
		UpstreamType: &v1.Upstream_Kube{
			Kube: &kubeplugin.UpstreamSpec{
				ServiceName:      meta.Name,
				ServiceNamespace: meta.Namespace,
				ServicePort:      uint32(port.Port),
				Selector:         labels,
			},
		},
		DiscoveryMetadata: &v1.DiscoveryMetadata{},
	}

	for _, sc := range uc.serviceConverters {
		if err := sc.ConvertService(svc, port, us); err != nil {
			contextutils.LoggerFrom(ctx).Errorf("error: failed to process service options with err %v", err)
		}
	}

	return us
}

func UpstreamName(serviceNamespace, serviceName string, servicePort int32, extraLabels map[string]string) string {

	var labelsTag string
	if len(extraLabels) > 0 {
		_, values := keysAndValues(extraLabels)
		labelsTag = fmt.Sprintf("-%v", strings.Join(values, "-"))
	}
	return sanitizer.SanitizeNameV2(fmt.Sprintf("%s-%s%s-%v", serviceNamespace, serviceName, labelsTag, servicePort))
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

func UpdateUpstream(original, desired *v1.Upstream) (didChange bool, err error) {
	originalSpec, ok := original.UpstreamType.(*v1.Upstream_Kube)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.Upstream_Kube, got %v", reflect.TypeOf(original.UpstreamType).Name())
	}
	desiredSpec, ok := desired.UpstreamType.(*v1.Upstream_Kube)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.Upstream_Kube, got %v", reflect.TypeOf(original.UpstreamType).Name())
	}
	// copy service spec, we don't want to overwrite that
	desiredSpec.Kube.ServiceSpec = originalSpec.Kube.ServiceSpec
	// copy labels; user may have written them over. cannot be auto-discovered
	desiredSpec.Kube.Selector = originalSpec.Kube.Selector

	utils.UpdateUpstream(original, desired)

	return !upstreamsEqual(original, desired), nil
}

// we want to know if the upstreams are equal apart from their Status and Metadata
func upstreamsEqual(original, desired *v1.Upstream) bool {
	copyOriginal := *original
	copyDesired := *desired

	copyOriginal.Metadata = copyDesired.Metadata
	copyOriginal.Status = copyDesired.Status

	return copyOriginal.Equal(copyDesired)
}
