package upstreams

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/rotisserie/eris"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func isRealUpstream(upstreamName string) bool {
	return !(kubernetes.IsKubeUpstream(upstreamName) || consul.IsConsulUpstream(upstreamName))
}

func DestinationToUpstreamRef(dest *v1.Destination) (*core.ResourceRef, error) {
	var ref *core.ResourceRef
	switch d := dest.GetDestinationType().(type) {
	case *v1.Destination_Upstream:
		ref = d.Upstream
	case *v1.Destination_Kube:
		ref = kubernetes.DestinationToUpstreamRef(d.Kube)
	case *v1.Destination_Consul:
		ref = consul.DestinationToUpstreamRef(d.Consul)
	case *v1.Destination_DynamicForwardProxy:
		// other upstreams will exist in memory, e.g. https://github.com/solo-io/gloo/blob/46cefb3f3ad026b1430ef0003e2861e89d526b4d/projects/gloo/pkg/upstreams/kubernetes/upstream_client.go#L44
		// but no upstream exists, even in memory, for dynamic forward proxy
		return nil, eris.Errorf("logic error")
		//ref = &core.ResourceRef{
		//	Name:      "placeholder",
		//	Namespace: "gloo-system",
		//}
		// TODO(kdorosH)
		//ref = consul.DestinationToUpstreamRef(d.Consul)
	default:
		return nil, eris.Errorf("no destination type specified")
	}
	return ref, nil
}
