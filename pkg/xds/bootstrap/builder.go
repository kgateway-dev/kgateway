package bootstrap

import (
	"fmt"
	"slices"

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
MIIC9jCCAd4CCQDziJBJLFeNxDANBgkqhkiG9w0BAQsFADA8MScwJQYDVQQDDB5r
Z2F0ZXdheS1zeXN0ZW0tY2EtcGxhY2Vob2xkZXIxETAPBgNVBAoMCGtnYXRld2F5
MCAXDTI2MDUxODEyNDMzOFoYDzIxMjYwNTE5MTI0MzM4WjA8MScwJQYDVQQDDB5r
Z2F0ZXdheS1zeXN0ZW0tY2EtcGxhY2Vob2xkZXIxETAPBgNVBAoMCGtnYXRld2F5
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyIhEq9QW2+mRxrKC7cdG
hIyQYNepOdkdFmlA8Aj7e06Rw+qrZ/nt0b8UFyavbgYASa4zRcVOFoFz/bOeQ+AO
1jqYCIrPZFByuKMZMoXowqzMU/NJxFvjVWEtFCAWa5+Saf/IiKrVT3ra+lFd+oIy
/P8jO05wMRoNO2ZrT+Jc5AH6JQ7bV8mk5k35TUO1JkCjONpW/IgGgBIyeglpXU2a
J2is4EewbOOuPnmhmSTHR4Sf3y8aa18AXDbEIfuCJtQSJm5dEGI2Kr1DsQQLqklK
tw5kG2z4z43G0jCf/H04TzUyrIO5c/Q6bzkpawHLWVyqfQpPLZ7taQLjqZ9zs/rR
6QIDAQABMA0GCSqGSIb3DQEBCwUAA4IBAQC9huFeWp+0nPEor2BK5zVVzBz4FSdX
oIZv7rbZY4OMZaJd9Z6y07aj/YM0kx66YqoIe5X0kA4NDLe8VST6kYecugQ6HeIL
B70kEZlc8X5q4xjOj/t0EJOsmP0hZK+6aNvjr6x1rm0fWvwFUbNvLK0SKkVq/urI
FwD8n0mhM0tkPjpsVMnhx7NO98dwKASAPk/eBdQHP3L4nNAt+eOp042OeRmQOyOx
FHW4vPA/rQpONzRjk0fch5/sMDzspQsf/EQpl++MT9X3QC0mCf1z8T2qzjn72SJ7
yitqAQ59a80qeeQ8i3nAI5clnJtfDYwZV6gIO72hygBWWE5FMjWzGPCE
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

// SystemCAValidationSecretPlaceholder returns a secret with a placeholder CA certificate.
// This is used so that strict validation won't erroneously fail for backend TLS configs that reference system CA certificates,
// that would otherwise be missing in the validation bootstrap config.
func SystemCAValidationSecretPlaceholder() *envoytlsv3.Secret {
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

func clustersReferenceSystemCASecret(clusters []*envoyclusterv3.Cluster) bool {
	return slices.ContainsFunc(clusters, ClusterReferencesSystemCASecret)
}

func hasSecretNamed(secrets []*envoytlsv3.Secret, name string) bool {
	for _, secret := range secrets {
		if secret != nil && secret.GetName() == name {
			return true
		}
	}
	return false
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
		staticResources.Secrets = append(staticResources.Secrets, b.secrets...)
	}
	if clustersReferenceSystemCASecret(b.clusters) && !hasSecretNamed(staticResources.GetSecrets(), eiutils.SystemCaSecretName) {
		staticResources.Secrets = append(staticResources.Secrets, SystemCAValidationSecretPlaceholder())
	}

	return &envoybootstrapv3.Bootstrap{
		Node: &envoycorev3.Node{
			Id:      "validation-node-id",
			Cluster: "validation-cluster",
		},
		StaticResources: staticResources,
	}, nil
}
