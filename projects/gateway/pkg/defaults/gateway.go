package defaults

import (
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func DefaultGateway(writeNamespace string) *v2.Gateway {
	return &v2.Gateway{
		Metadata: core.Metadata{
			Name:        "gateway",
			Namespace:   writeNamespace,
			Annotations: map[string]string{defaults.OriginKey: defaults.DefaultValue},
		},
		GatewayProxyName: translator.GatewayProxyName,
		GatewayType: &v2.Gateway_HttpGateway{
			HttpGateway: &v2.HttpGateway{},
		},
		BindAddress:   "::",
		BindPort:      defaults.HttpPort,
		UseProxyProto: &types.BoolValue{Value: false},
	}
}

func DefaultSslGateway(writeNamespace string) *v2.Gateway {
	defaultgw := DefaultGateway(writeNamespace)
	defaultgw.Metadata.Name = defaultgw.Metadata.Name + "-ssl"
	defaultgw.BindPort = defaults.HttpsPort
	defaultgw.Ssl = true

	return defaultgw
}

// The default TCP gateways are currently only used for testing purposes
// but could be included later if we decide they should be.
func DefaultTcpGateway(writeNamespace string) *v2.Gateway {
	return &v2.Gateway{
		Metadata: core.Metadata{
			Name:        "gateway-tcp",
			Namespace:   writeNamespace,
			Annotations: map[string]string{defaults.OriginKey: defaults.DefaultValue},
		},
		GatewayType: &v2.Gateway_TcpGateway{
			TcpGateway: &v2.TcpGateway{},
		},
		GatewayProxyName: translator.GatewayProxyName,
		BindAddress:      "::",
		BindPort:         defaults.TcpPort,
		UseProxyProto:    &types.BoolValue{Value: false},
	}
}

func DefaultTcpSslGateway(writeNamespace string) *v2.Gateway {
	defaultgw := DefaultTcpGateway(writeNamespace)
	defaultgw.Metadata.Name = defaultgw.Metadata.Name + "-ssl"
	defaultgw.BindPort = defaults.HttpsPort
	defaultgw.Ssl = true

	return defaultgw
}

func DefaultVirtualService(namespace, name string) *v1.VirtualService {
	return &v1.VirtualService{
		Metadata: core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		VirtualHost: &gloov1.VirtualHost{
			Name:    "routes",
			Domains: []string{"*"},
			Routes: []*gloov1.Route{{
				Matcher: &gloov1.Matcher{
					PathSpecifier: &gloov1.Matcher_Prefix{Prefix: "/"},
				},
				Action: &gloov1.Route_DirectResponseAction{DirectResponseAction: &gloov1.DirectResponseAction{
					Status: 200,
					Body: `Gloo and Envoy are configured correctly!

Delete the '` + name + ` Virtual Service to get started. 	
`,
				}},
			}},
		},
	}
}

func LocalUpstream(writeNamespace string) *gloov1.Upstream {
	return &gloov1.Upstream{
		Metadata: core.Metadata{
			Name:      "upstream",
			Namespace: writeNamespace,
		},
		UpstreamSpec: &gloov1.UpstreamSpec{},
	}
}
