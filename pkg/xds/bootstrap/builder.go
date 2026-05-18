package bootstrap

import (
	"fmt"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"

	eiutils "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const systemCAValidationPlaceholderCert = `-----BEGIN CERTIFICATE-----
MIIC1jCCAb4CCQCJczLyBBZ1GTANBgkqhkiG9w0BAQsFADAtMRUwEwYDVQQKDAxl
eGFtcGxlIEluYy4xFDASBgNVBAMMC2V4YW1wbGUuY29tMB4XDTI1MDMwNzE0Mjkx
NloXDTI2MDMwNzE0MjkxNlowLTEVMBMGA1UECgwMZXhhbXBsZSBJbmMuMRQwEgYD
VQQDDAtleGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
AN0U6TVYECkwqnxh1Kt3dS+LialrXBOXKagj9tE582T6dwmqThD75VZPrNKkRoYO
aUzCctfDkUBXRemOTMut7ES5xoAtSAhr2GAnqgM3+yBCLOxooSjEFdlpFT7dhi1w
jOPa5iMh6ve/pHuRHvEuaF/J6P8tr83wGutx/xFZVuGA9V1AmBmYhePM+JhdcwaB
1+IbJp30gGyPfY4vdRQ9VQWbThE8psEzah+3SgTKJSIT7NAdwiIu3O3rXORbaYYU
oycgXUHdOKRbJnbvy3pTnFZJ50sg1HIA4yBdX7c0diy8Zz3Suoondg3DforWr0pB
Hs6tySAQoz2RiAqDqcE2rbMCAwEAATANBgkqhkiG9w0BAQsFAAOCAQEAWPkz3dJW
b+LFtnv7MlOVM79Y4PqeiHnazP1G9FwnWBHARkjISsax3b0zX8/RHnU83c3tLP5D
VwenYb9B9mzXbLiWI8aaX0UXP//D593ti15y0Od7yC2hQszlqIbxYnkFVwXoT9fQ
bdQ9OtpCt8EZnKEyCxck+hlKEyYTcH2PqZ7Ndp0M8I2znz3Kut/uYHLUddfoPF/m
O0V6fbyB/Mx/G1uLiv/BVpx3AdP+3ygJyKtelXkD+IdlY3y110fzmVr6NgxAbz/h
n9KpuK4SEloIycZUaKVXAaX7T42SFYw7msmB+Uu7z5oLOijsjX6TjeofdFBZ/Byl
SxODgqhtaPnOxQ==
-----END CERTIFICATE-----`

// ConfigBuilder helps construct a partial bootstrap config for validation.
type ConfigBuilder struct {
	filterConfigs ir.TypedFilterConfigMap
	routes        []*envoyroutev3.Route
	clusters      []*envoyclusterv3.Cluster
	secrets       []*envoytlsv3.Secret
	httpFilters   []*envoy_extensions_filters_network_http_connection_manager_v3.HttpFilter
}

// New creates a new ConfigBuilder.
func New() *ConfigBuilder {
	return &ConfigBuilder{
		filterConfigs: make(ir.TypedFilterConfigMap),
	}
}

// AddFilterConfig adds a filter configuration to the builder. Assumes that the
// filter config is a valid proto message and error handling is done by the caller.
func (b *ConfigBuilder) AddFilterConfig(name string, config proto.Message) {
	b.filterConfigs.AddTypedConfig(name, config)
}

// AddRoute adds a route to the builder.
func (b *ConfigBuilder) AddRoute(route *envoyroutev3.Route) {
	b.routes = append(b.routes, route)
}

// AddCluster adds a cluster to the builder.
func (b *ConfigBuilder) AddCluster(cluster *envoyclusterv3.Cluster) {
	b.clusters = append(b.clusters, cluster)
}

// AddSecret adds a static secret to the bootstrap.
func (b *ConfigBuilder) AddSecret(secret *envoytlsv3.Secret) {
	b.secrets = append(b.secrets, secret)
}

func SystemCAValidationSecret() *envoytlsv3.Secret {
	return &envoytlsv3.Secret{
		Name: eiutils.SystemCaSecretName,
		Type: &envoytlsv3.Secret_ValidationContext{
			ValidationContext: &envoytlsv3.CertificateValidationContext{
				TrustedCa: &envoycorev3.DataSource{
					Specifier: &envoycorev3.DataSource_InlineString{
						InlineString: systemCAValidationPlaceholderCert,
					},
				},
			},
		},
	}
}

func ClusterReferencesSystemCASecret(cluster *envoyclusterv3.Cluster) bool {
	if cluster == nil || cluster.GetTransportSocket() == nil || cluster.GetTransportSocket().GetTypedConfig() == nil {
		return false
	}

	upstreamTLS := &envoytlsv3.UpstreamTlsContext{}
	if err := cluster.GetTransportSocket().GetTypedConfig().UnmarshalTo(upstreamTLS); err != nil {
		return false
	}

	commonTLS := upstreamTLS.GetCommonTlsContext()
	if commonTLS == nil {
		return false
	}

	switch validation := commonTLS.GetValidationContextType().(type) {
	case *envoytlsv3.CommonTlsContext_CombinedValidationContext:
		return validation.CombinedValidationContext.GetValidationContextSdsSecretConfig().GetName() == eiutils.SystemCaSecretName
	case *envoytlsv3.CommonTlsContext_ValidationContextSdsSecretConfig:
		return validation.ValidationContextSdsSecretConfig.GetName() == eiutils.SystemCaSecretName
	default:
		return false
	}
}

// AddHttpFilter adds an HTTP filter to the HCM filter chain.
func (b *ConfigBuilder) AddHttpFilter(filter *envoy_extensions_filters_network_http_connection_manager_v3.HttpFilter) {
	b.httpFilters = append(b.httpFilters, filter)
}

// Build creates a partial bootstrap config suitable for validation.
func (b *ConfigBuilder) Build() (*envoybootstrapv3.Bootstrap, error) {
	vhost := &envoyroutev3.VirtualHost{
		Name:    "placeholder_vhost",
		Domains: []string{"*"},
	}
	if len(b.filterConfigs) > 0 {
		vhost.TypedPerFilterConfig = b.filterConfigs.ToAnyMap()
	}
	if len(b.routes) > 0 {
		vhost.Routes = b.routes
	}

	hcm := &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
		StatPrefix: "placeholder",
		RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig{
			RouteConfig: &envoyroutev3.RouteConfiguration{
				VirtualHosts: []*envoyroutev3.VirtualHost{vhost},
			},
		},
	}

	// Add HTTP filters if present
	if len(b.httpFilters) > 0 {
		hcm.HttpFilters = b.httpFilters
	}

	// Always add router filter at the end (required by Envoy)
	routerAny, err := utils.MessageToAny(&envoyhttpv3.Router{})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Router filter: %w", err)
	}
	hcm.HttpFilters = append(hcm.HttpFilters, &envoy_extensions_filters_network_http_connection_manager_v3.HttpFilter{
		Name: envoywellknown.Router,
		ConfigType: &envoy_extensions_filters_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: routerAny,
		},
	})

	hcmAny, err := utils.MessageToAny(hcm)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal HttpConnectionManager: %w", err)
	}

	staticResources := &envoybootstrapv3.Bootstrap_StaticResources{
		Listeners: []*envoylistenerv3.Listener{{
			Name: "placeholder_listener",
			Address: &envoycorev3.Address{
				Address: &envoycorev3.Address_SocketAddress{
					SocketAddress: &envoycorev3.SocketAddress{
						Address:       "0.0.0.0",
						PortSpecifier: &envoycorev3.SocketAddress_PortValue{PortValue: 8081},
					},
				},
			},
			FilterChains: []*envoylistenerv3.FilterChain{{
				Name: "placeholder_filter_chain",
				Filters: []*envoylistenerv3.Filter{{
					Name: envoywellknown.HTTPConnectionManager,
					ConfigType: &envoylistenerv3.Filter_TypedConfig{
						TypedConfig: hcmAny,
					},
				}},
			}},
		}},
	}
	if len(b.clusters) > 0 {
		staticResources.Clusters = b.clusters
	}
	if len(b.secrets) > 0 {
		staticResources.Secrets = b.secrets
	}

	return &envoybootstrapv3.Bootstrap{
		Node: &envoycorev3.Node{
			Id:      "validation-node-id",
			Cluster: "validation-cluster",
		},
		StaticResources: staticResources,
	}, nil
}
