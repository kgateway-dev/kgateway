package conversion

import (
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
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
	return &gatewayv2.Gateway{
		Metadata: core.Metadata{
			Namespace:   src.GetMetadata().Namespace,
			Name:        src.GetMetadata().Name,
			Annotations: map[string]string{defaults.OriginKey: defaults.ConvertedValue},
		},
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
		GatewayProxyName: translator.GatewayProxyName,
	}
}
