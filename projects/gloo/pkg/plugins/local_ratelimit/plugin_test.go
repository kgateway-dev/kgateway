package local_ratelimit

import (
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyratelimit "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	envoy_extensions_filters_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_extensions_filters_network_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/local_ratelimit/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	local_ratelimit "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/local_ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var _ = Describe("Local Rate Limit Plugin", func() {
	Context("Copies the local rate limit config from the CR to the filter", func() {
		var p *plugin
		var httpListener *v1.HttpListener
		var tokenBucket *local_ratelimit.TokenBucket
		var expectedFilter *envoy_extensions_filters_http_local_ratelimit_v3.LocalRateLimit

		BeforeEach(func() {
			p = NewPlugin()
			p.Init(plugins.InitParams{})

			httpListener = &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					HttpLocalRatelimit: &local_ratelimit.Settings{
						LocalRateLimitPerDownstreamConnection: true,
						EnableXRatelimitHeaders:               true,
						Defaults: &local_ratelimit.TokenBucket{
							MaxTokens: 10,
							TokensPerFill: &wrapperspb.UInt32Value{
								Value: 10,
							},
							FillInterval: &durationpb.Duration{
								Seconds: 10,
							},
						},
					},
				},
			}

			tokenBucket = &local_ratelimit.TokenBucket{
				MaxTokens: 10,
				TokensPerFill: &wrapperspb.UInt32Value{
					Value: 10,
				},
				FillInterval: &durationpb.Duration{
					Seconds: 10,
				},
			}

			expectedFilter = &envoy_extensions_filters_http_local_ratelimit_v3.LocalRateLimit{
				StatPrefix:                            HTTPFilterStatPrefix,
				LocalRateLimitPerDownstreamConnection: true,
				EnableXRatelimitHeaders:               envoyratelimit.XRateLimitHeadersRFCVersion_DRAFT_VERSION_03,
				Stage:                                 CustomStageBeforeAuth,
				FilterEnabled: &corev3.RuntimeFractionalPercent{
					DefaultValue: &envoy_type_v3.FractionalPercent{
						Numerator:   100,
						Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
					},
				},
				FilterEnforced: &corev3.RuntimeFractionalPercent{
					DefaultValue: &envoy_type_v3.FractionalPercent{
						Numerator:   100,
						Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
					},
				},
				TokenBucket: &envoy_type_v3.TokenBucket{
					MaxTokens: 10,
					TokensPerFill: &wrapperspb.UInt32Value{
						Value: 10,
					},
					FillInterval: &durationpb.Duration{
						Seconds: 10,
					},
				},
			}
		})

		It("Copies the l4 local rate limit config from the listener to the filter", func() {
			filters, err := p.NetworkFiltersHTTP(plugins.Params{}, &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					NetworkLocalRatelimit: tokenBucket,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			typedConfig, err := utils.MessageToAny(&envoy_extensions_filters_network_local_ratelimit_v3.LocalRateLimit{
				StatPrefix: NetworkFilterStatPrefix,
				TokenBucket: &envoy_type_v3.TokenBucket{
					MaxTokens: 10,
					TokensPerFill: &wrapperspb.UInt32Value{
						Value: 10,
					},
					FillInterval: &durationpb.Duration{
						Seconds: 10,
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(Equal([]plugins.StagedNetworkFilter{
				{
					NetworkFilter: &envoy_config_listener_v3.Filter{
						Name: NetworkFilterName,
						ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
							TypedConfig: typedConfig,
						},
					},
					Stage: pluginStage,
				},
			}))
		})

		It("Copies the http local rate limit config from the HTTP Listener to the filter", func() {
			filters, err := p.HttpFilters(plugins.Params{}, httpListener)
			Expect(err).NotTo(HaveOccurred())
			typedConfig, err := utils.MessageToAny(expectedFilter)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(Equal([]plugins.StagedHttpFilter{
				{
					HttpFilter: &envoyhcm.HttpFilter{
						Name: HTTPFilterName,
						ConfigType: &envoyhcm.HttpFilter_TypedConfig{
							TypedConfig: typedConfig,
						},
					},
					Stage: pluginStage,
				},
			}))
		})

		It("Copies the http local rate limit config from the virtual host to the filter", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			err := p.ProcessVirtualHost(plugins.VirtualHostParams{
				HttpListener: httpListener,
			}, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					RateLimitConfigType: &v1.VirtualHostOptions_Ratelimit{
						Ratelimit: &ratelimit.RateLimitVhostExtension{
							LocalRatelimit: tokenBucket,
						},
					},
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			typedConfig, err := utils.MessageToAny(expectedFilter)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal(&envoy_config_route_v3.VirtualHost{
				TypedPerFilterConfig: map[string]*anypb.Any{
					HTTPFilterName: typedConfig,
				},
			}))
		})

		It("Copies the http local rate limit config from the route to the filter", func() {
			out := &envoy_config_route_v3.Route{}
			err := p.ProcessRoute(plugins.RouteParams{
				VirtualHostParams: plugins.VirtualHostParams{
					HttpListener: httpListener,
				},
			}, &v1.Route{
				Options: &v1.RouteOptions{
					RateLimitConfigType: &v1.RouteOptions_Ratelimit{
						Ratelimit: &ratelimit.RateLimitRouteExtension{
							LocalRatelimit: tokenBucket,
						},
					},
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			typedConfig, err := utils.MessageToAny(expectedFilter)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal(&envoy_config_route_v3.Route{
				TypedPerFilterConfig: map[string]*anypb.Any{
					HTTPFilterName: typedConfig,
				},
			}))
		})
	})
})
