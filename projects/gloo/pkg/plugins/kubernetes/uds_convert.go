package kubernetes

import (
	"context"
	"fmt"
	"reflect"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes/serviceconverter"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"

	sanitizer "github.com/solo-io/k8s-utils/kubeutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
)

type UpstreamConverter interface {
	UpstreamsForService(ctx context.Context, svc *kubev1.Service) v1.UpstreamList
}

func DefaultUpstreamConverter() *KubeUpstreamConverter {
	kuc := new(KubeUpstreamConverter)
	kuc.serviceConverters = serviceconverter.DefaultServiceConverters
	return kuc
}

type KubeUpstreamConverter struct {
	serviceConverters []serviceconverter.ServiceConverter
}

func (uc *KubeUpstreamConverter) UpstreamsForService(ctx context.Context, svc *kubev1.Service) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for _, port := range svc.Spec.Ports {
		upstreams = append(upstreams, uc.CreateUpstream(ctx, svc, port))
	}
	return upstreams
}

func (uc *KubeUpstreamConverter) CreateUpstream(ctx context.Context, svc *kubev1.Service, port kubev1.ServicePort) *v1.Upstream {
	meta := svc.ObjectMeta
	coremeta := kubeutils.FromKubeMeta(meta)
	coremeta.ResourceVersion = ""
	coremeta.Name = UpstreamName(meta.Namespace, meta.Name, port.Port)
	labels := coremeta.Labels
	coremeta.Labels = make(map[string]string)

	us := &v1.Upstream{
		Metadata: coremeta,
		UpstreamType: &v1.Upstream_Kube{
			Kube: &kubeplugin.UpstreamSpec{
				ServiceName:      meta.Name,
				ServiceNamespace: meta.Namespace,
				ServicePort:      uint32(port.Port),
				Selector:         svc.Spec.Selector,
			},
		},
		DiscoveryMetadata: &v1.DiscoveryMetadata{
			Labels: labels,
		},
	}

	for _, sc := range uc.serviceConverters {
		if err := sc.ConvertService(svc, port, us); err != nil {
			contextutils.LoggerFrom(ctx).Errorf("error: failed to process service options with err %v", err)
		}
	}

	return us
}

func UpstreamName(serviceNamespace, serviceName string, servicePort int32) string {
	return sanitizer.SanitizeNameV2(fmt.Sprintf("%s-%s-%v", serviceNamespace, serviceName, servicePort))
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
	originalSpec, ok := original.GetUpstreamType().(*v1.Upstream_Kube)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.Upstream_Kube, got %v", reflect.TypeOf(original.GetUpstreamType()).Name())
	}
	desiredSpec, ok := desired.GetUpstreamType().(*v1.Upstream_Kube)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.Upstream_Kube, got %v", reflect.TypeOf(original.GetUpstreamType()).Name())
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
