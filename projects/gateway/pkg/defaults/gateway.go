package defaults

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
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
	defaultgw.Metadata.Name = defaultgw.Metadata.Name + "-ssl"
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
	defaultgw.Metadata.Name = defaultgw.Metadata.Name + "-ssl"
	defaultgw.BindPort = defaults.HttpsPort
	defaultgw.Ssl = true

	return defaultgw
}

func DefaultVirtualService(namespace, name string) *v1.VirtualService {
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
					Body: `Gloo and Envoy are configured correctly!

Delete the '` + name + ` Virtual Service to get started. 	
`,
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
