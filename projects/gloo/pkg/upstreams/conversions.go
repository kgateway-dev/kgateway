package upstreams

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/go-utils/errors"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubepluginapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
)

// Contains invalid character so any accidental attempt to write to storage fails
const ServiceUpstreamNamePrefix = "svc:"

func isRealUpstream(upstreamName string) bool {
	return !strings.HasPrefix(upstreamName, ServiceUpstreamNamePrefix)
}

func DestinationToUpstreamRef(dest *v1.Destination) (*core.ResourceRef, error) {
	var ref *core.ResourceRef
	switch d := dest.DestinationType.(type) {
	case *v1.Destination_Upstream:
		ref = d.Upstream
	case *v1.Destination_Service:
		ref = serviceDestinationToUpstreamRef(d.Service)
	default:
		return nil, errors.Errorf("no destination type specified")
	}
	return ref, nil
}

func serviceDestinationToUpstreamRef(svcDest *v1.ServiceDestination) *core.ResourceRef {
	return &core.ResourceRef{
		Namespace: svcDest.Ref.Namespace,
		Name:      buildFakeUpstreamName(svcDest.Ref.Name, int32(svcDest.Port)),
	}
}

func buildFakeUpstreamName(serviceName string, port int32) string {
	return fmt.Sprintf("%s%s-%d", ServiceUpstreamNamePrefix, serviceName, port)
}

func reconstructServiceName(fakeUpstreamName string) (string, int32, error) {
	noPrefix := strings.TrimPrefix(fakeUpstreamName, ServiceUpstreamNamePrefix)
	namePortSeparatorIndex := strings.LastIndex(noPrefix, "-")
	serviceName := noPrefix[:namePortSeparatorIndex]
	portStr := noPrefix[namePortSeparatorIndex+1:]
	portInt64, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return "", 0, errors.Wrapf(err, "service-derived upstream has malformed name: %s", fakeUpstreamName)
	}
	return serviceName, int32(portInt64), nil
}

func servicesToUpstreams(services skkube.ServiceList) v1.UpstreamList {
	var result v1.UpstreamList
	for _, svc := range services {
		for _, port := range svc.Spec.Ports {
			kubeSvc := kubev1.Service(svc.Service)
			result = append(result, serviceToUpstream(&kubeSvc, port))
		}
	}
	return result
}

func serviceToUpstream(svc *kubev1.Service, port kubev1.ServicePort) *gloov1.Upstream {
	coreMeta := kubeutils.FromKubeMeta(svc.ObjectMeta)

	coreMeta.Name = buildFakeUpstreamName(svc.Name, port.Port)
	coreMeta.Namespace = svc.Namespace
	coreMeta.ResourceVersion = ""

	return &gloov1.Upstream{
		Metadata: coreMeta,
		UpstreamSpec: &v1.UpstreamSpec{
			UpstreamType: &v1.UpstreamSpec_Kube{
				Kube: &kubepluginapi.UpstreamSpec{
					ServiceSpec:      kubeplugin.GetServiceSpec(svc, port),
					ServiceName:      svc.Name,
					ServiceNamespace: svc.Namespace,
					ServicePort:      uint32(port.Port),
				},
			},
		},
		DiscoveryMetadata: &v1.DiscoveryMetadata{},
	}
}
