package conversion

import (
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type GatewayConverter interface {
	FromV1ToV2(src *gatewayv1.Gateway) *gatewayv2.Gateway
}

type gatewayConverter struct{}

func NewGatewayConverter() GatewayConverter {
	return &gatewayConverter{}
}

func (c *gatewayConverter) FromV1ToV2(src *gatewayv1.Gateway) *gatewayv2.Gateway {
	srcMeta := src.GetMetadata()
	dstMeta := core.Metadata{
		Namespace:   srcMeta.Namespace,
		Name:        srcMeta.Name,
		Cluster:     srcMeta.Cluster,
		Labels:      srcMeta.Labels,
		Annotations: srcMeta.Annotations,
	}

	if dstMeta.Annotations == nil {
		dstMeta.Annotations = make(map[string]string, 1)
	}
	dstMeta.Annotations[defaults.OriginKey] = defaults.ConvertedValue

	return &gatewayv2.Gateway{
		Metadata:      dstMeta,
		Ssl:           src.Ssl,
		BindAddress:   src.BindAddress,
		BindPort:      src.BindPort,
		UseProxyProto: src.UseProxyProto,
		GatewayType: &gatewayv2.Gateway_HttpGateway{
			HttpGateway: &gatewayv2.HttpGateway{
				VirtualServices: src.VirtualServices,
				Plugins:         src.Plugins,
			},
		},
		GatewayProxyName: gatewaydefaults.GatewayProxyName,
	}
}
