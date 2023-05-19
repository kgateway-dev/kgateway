package defaults

import (
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	GatewayProxyName        = "gateway-proxy"
	GatewayBindAddress      = "::"
	ConfigDumpServiceSuffix = "-config-dump-service"
)

func DefaultGateway(writeNamespace string) *v1.Gateway {
	return &v1.Gateway{
		Metadata: &core.Metadata{
			Name:        GatewayProxyName,
			Namespace:   writeNamespace,
			Annotations: map[string]string{defaults.OriginKey: defaults.DefaultValue},
		},
		ProxyNames: []string{GatewayProxyName},
		GatewayType: &v1.Gateway_HttpGateway{
			HttpGateway: &v1.HttpGateway{},
		},
		BindAddress:   GatewayBindAddress,
		BindPort:      defaults.HttpPort,
		UseProxyProto: &wrappers.BoolValue{Value: false},
	}
}

func DefaultSslGateway(writeNamespace string) *v1.Gateway {
	defaultgw := DefaultGateway(writeNamespace)
	defaultgw.GetMetadata().Name = defaultgw.GetMetadata().GetName() + "-ssl"
	defaultgw.BindPort = defaults.HttpsPort
	defaultgw.Ssl = true

	return defaultgw
}

// The default TCP gateways are currently only used for testing purposes
// but could be included later if we decide they should be.
func DefaultTcpGateway(writeNamespace string) *v1.Gateway {
	return &v1.Gateway{
		Metadata: &core.Metadata{
			Name:        "gateway-tcp",
			Namespace:   writeNamespace,
			Annotations: map[string]string{defaults.OriginKey: defaults.DefaultValue},
		},
		GatewayType: &v1.Gateway_TcpGateway{
			TcpGateway: &v1.TcpGateway{},
		},
		ProxyNames:    []string{GatewayProxyName},
		BindAddress:   GatewayBindAddress,
		BindPort:      defaults.TcpPort,
		UseProxyProto: &wrappers.BoolValue{Value: false},
	}
}

func DefaultTcpSslGateway(writeNamespace string) *v1.Gateway {
	defaultgw := DefaultTcpGateway(writeNamespace)
	defaultgw.GetMetadata().Name = defaultgw.GetMetadata().GetName() + "-ssl"
	defaultgw.BindPort = defaults.HttpsPort
	defaultgw.Ssl = true

	return defaultgw
}

// The default Hybrid gateway is currently only used for testing purposes
// but could be included later if we decide it should be.
func DefaultHybridGateway(writeNamespace string) *v1.Gateway {
	return &v1.Gateway{
		Metadata: &core.Metadata{
			Name:        GatewayProxyName + "-hybrid",
			Namespace:   writeNamespace,
			Annotations: map[string]string{defaults.OriginKey: defaults.DefaultValue},
		},
		GatewayType: &v1.Gateway_HybridGateway{
			HybridGateway: &v1.HybridGateway{
				MatchedGateways: []*v1.MatchedGateway{
					{
						GatewayType: &v1.MatchedGateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
					},
				},
			},
		},
		ProxyNames:    []string{GatewayProxyName},
		BindAddress:   GatewayBindAddress,
		BindPort:      defaults.HybridPort,
		UseProxyProto: &wrappers.BoolValue{Value: false},
	}
}

func DefaultHybridSslGateway(writeNamespace string) *v1.Gateway {
	gw := DefaultHybridGateway(writeNamespace)
	gw.GatewayType = &v1.Gateway_HybridGateway{
		HybridGateway: &v1.HybridGateway{
			MatchedGateways: []*v1.MatchedGateway{
				{
					Matcher: &v1.Matcher{
						// Define a non-nil SslConfig
						SslConfig: &ssl.SslConfig{
							TransportSocketConnectTimeout: &duration.Duration{
								Seconds: 30,
							},
						},
					},
					GatewayType: &v1.MatchedGateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{},
					},
				},
			},
		},
	}

	return gw
}

func DefaultMatchableHttpGateway(writeNamespace string, sslConfigMatch *ssl.SslConfig) *v1.MatchableHttpGateway {
	return &v1.MatchableHttpGateway{
		Metadata: &core.Metadata{
			Name:        "matchable-http-gateway",
			Namespace:   writeNamespace,
			Annotations: map[string]string{defaults.OriginKey: defaults.DefaultValue},
		},
		Matcher: &v1.MatchableHttpGateway_Matcher{
			SslConfig: sslConfigMatch,
		},
		HttpGateway: &v1.HttpGateway{
			// select all virtual services
		},
	}
}

func DefaultVirtualService(namespace, name string) *v1.VirtualService {
	return DirectResponseVirtualService(namespace, name, `Gloo and Envoy are configured correctly!

	Delete the '`+name+` Virtual Service to get started. 	
	`)
}

func DirectResponseVirtualService(namespace, name, body string) *v1.VirtualService {
	return &v1.VirtualService{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		VirtualHost: &v1.VirtualHost{
			Domains: []string{"*"},
			Routes: []*v1.Route{{
				Matchers: []*matchers.Matcher{DefaultMatcher()},
				Action: &v1.Route_DirectResponseAction{DirectResponseAction: &gloov1.DirectResponseAction{
					Status: 200,
					Body:   body,
				}},
			}},
		},
	}
}

func DefaultMatcher() *matchers.Matcher {
	return &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/"}}
}

func GatewayProxyConfigDumpServiceName(name string) string {
	return name + ConfigDumpServiceSuffix
}
