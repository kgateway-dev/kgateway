package kubernetes

import (
	"strings"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubepluginapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
)

func IsKubeUpstream(upstreamName string) bool {
	return strings.HasPrefix(upstreamName, upstreamNamePrefix)
}

func DestinationToUpstreamRef(svcDest *v1.KubernetesServiceDestination) *core.ResourceRef {
	return &core.ResourceRef{
		Namespace: svcDest.Ref.Namespace,
		Name:      fakeUpstreamName(svcDest.Ref.Name, svcDest.Ref.Namespace, int32(svcDest.Port)),
	}
}

func fakeUpstreamName(serviceName, serviceNamespace string, port int32) string {
	regularServiceName := kubeplugin.UpstreamName(serviceNamespace, serviceName, port, nil)
	return upstreamNamePrefix + regularServiceName
}

// Public because it's needed in the translator test
func KubeServicesToUpstreams(services skkube.ServiceList) v1.UpstreamList {
	var result v1.UpstreamList
	for _, svc := range services {
		for _, port := range svc.Spec.Ports {
			kubeSvc := svc.Service.Service
			result = append(result, serviceToUpstream(&kubeSvc, port))
		}
	}
	return result
}

func serviceToUpstream(svc *kubev1.Service, port kubev1.ServicePort) *gloov1.Upstream {
	coreMeta := kubeutils.FromKubeMeta(svc.ObjectMeta)

	coreMeta.Name = fakeUpstreamName(svc.Name, svc.Namespace, port.Port)
	coreMeta.Namespace = svc.Namespace
	coreMeta.ResourceVersion = ""

	return &gloov1.Upstream{
		Metadata: coreMeta,
		UpstreamSpec: &v1.UpstreamSpec{
			UseHttp2: kubeplugin.UseHttp2(svc, port),
			UpstreamType: &v1.UpstreamSpec_Kube{
				Kube: &kubepluginapi.UpstreamSpec{
					ServiceName:      svc.Name,
					ServiceNamespace: svc.Namespace,
					ServicePort:      uint32(port.Port),
				},
			},
		},
	}
}
