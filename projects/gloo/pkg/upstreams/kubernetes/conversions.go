package kubernetes

import (
	"context"
	"strings"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
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
func KubeServicesToUpstreams(ctx context.Context, services skkube.ServiceList) v1.UpstreamList {
	var result v1.UpstreamList
	for _, svc := range services {
		for _, port := range svc.Spec.Ports {
			kubeSvc := svc.Service.GetKubeService()
			result = append(result, serviceToUpstream(ctx, &kubeSvc, port))
		}
	}
	return result
}

func serviceToUpstream(ctx context.Context, svc *kubev1.Service, port kubev1.ServicePort) *gloov1.Upstream {
	us := kubeplugin.DefaultUpstreamConverter().CreateUpstream(ctx, svc, port, nil)

	us.Metadata.Name = fakeUpstreamName(svc.Name, svc.Namespace, port.Port)
	us.Metadata.Namespace = svc.Namespace
	us.Metadata.ResourceVersion = ""

	return us
}
