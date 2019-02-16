package kubernetes

import (
	"context"
	"crypto/md5"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/solo-io/solo-kit/pkg/utils/contextutils"

	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	discoveryAnnotationKey  = "gloo.solo.io/discover"
	discoveryAnnotationTrue = "true"
)

var ignoredLabels = []string{
	"pod-template-hash",        // it is common and provides nothing useful for discovery
	"controller-revision-hash", // set by helm
	"pod-template-generation",  // set by helm
	"release",                  // set by helm
}

func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	if p.kubeShareFactory == nil {
		p.kubeShareFactory = getInformerFactory(p.kube)
	}
	opts.Ctx = contextutils.WithLogger(opts.Ctx, "kube-uds")

	watch := p.kubeShareFactory.Subscribe()

	opts = opts.WithDefaults()
	upstreamsChan := make(chan v1.UpstreamList)
	errs := make(chan error)
	discoverUpstreams := func() {
		services, err := p.kubeShareFactory.ServicesLister().List(labels.SelectorFromSet(opts.Selector))
		if err != nil {
			errs <- err
			return
		}
		pods, err := p.kubeShareFactory.PodsLister().List(labels.SelectorFromSet(opts.Selector))
		if err != nil {
			errs <- err
			return
		}

		upstreamsChan <- convertServices(opts.Ctx, watchNamespaces, services, pods, discOpts, writeNamespace)
	}

	go func() {
		defer p.kubeShareFactory.Unsubscribe(watch)
		defer close(upstreamsChan)
		defer close(errs)
		// watch should open up with an initial read
		discoverUpstreams()
		for {
			select {
			case _, ok := <-watch:
				if !ok {
					return
				}
				discoverUpstreams()
			case <-opts.Ctx.Done():
				return
			}
		}
	}()
	return upstreamsChan, errs, nil
}

func convertServices(ctx context.Context, watchNamespaces []string, services []*kubev1.Service, pods []*kubev1.Pod, opts discovery.Opts, writeNamespace string) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for _, svc := range services {
		if skip(svc, opts) {
			continue
		}

		if !utils.AllNamespaces(watchNamespaces) {
			if !containsString(svc.Namespace, watchNamespaces) {
				continue
			}
		}

		upstreams = append(upstreams, upstreamsForService(ctx, svc, pods, writeNamespace)...)
	}
	return upstreams
}

func upstreamsForService(ctx context.Context, svc *kubev1.Service, pods []*kubev1.Pod, writeNamespace string) v1.UpstreamList {
	var upstreams v1.UpstreamList

	uniqueLabelSets := []map[string]string{
		svc.Spec.Selector,
	}
	if len(svc.Spec.Selector) > 0 {
		for _, pod := range pods {
			if pod.Namespace != svc.Namespace {
				continue
			}
			if !labels.AreLabelsInWhiteList(svc.Spec.Selector, pod.Labels) {
				continue
			}

			// create upstreams for the extra labels beyond the selector
			extendedLabels := make(map[string]string)
		addExtendedLabels:
			for k, v := range pod.Labels {
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

	for _, extendedLabels := range uniqueLabelSets {
		for _, port := range svc.Spec.Ports {
			upstreams = append(upstreams, createUpstream(ctx, writeNamespace, svc, port, extendedLabels))
		}
	}
	return upstreams
}

func createUpstream(ctx context.Context, writeNamespace string, svc *kubev1.Service, port kubev1.ServicePort, labels map[string]string) *v1.Upstream {
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
	coremeta.Name = strings.ToLower(upstreamName(meta.Namespace, meta.Name, port.Port, extraLabels))
	coremeta.Namespace = writeNamespace
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

func upstreamName(serviceNamespace, serviceName string, servicePort int32, extraLabels map[string]string) string {
	var labelsTag string
	if len(extraLabels) > 0 {
		_, values := keysAndValues(extraLabels)
		labelsTag = fmt.Sprintf("-%v", strings.Join(values, "-"))
	}
	name := fmt.Sprintf("%s-%s%s-%v", serviceNamespace, serviceName, labelsTag, servicePort)
	if len(name) > 63 {
		hash := md5.Sum([]byte(name))
		name = fmt.Sprintf("%s-%s-%v-%x", serviceNamespace, serviceName, servicePort, hash)
		// todo: ilackarms: handle potential collisions
		name = name[:63]
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

	if originalSpec.Equal(desiredSpec) {
		return false, nil
	}

	return true, nil
}
