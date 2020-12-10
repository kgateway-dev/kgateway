package csrf_test

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycsrf "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"

	"github.com/golang/protobuf/ptypes"

	api_type_matcher_v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	api_type_v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v31 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/csrf"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var _ = Describe("Plugin", func() {
	// TODO: why does global value change? -> FilterStage?

	It("copies the csrf config from the listener to the filter with AdditionalOrigins set", func() {
		apiAdditionalOrigins := []*api_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &api_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		envoyAdditionalOrigins := []*envoy_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Csrf: &v31.CsrfPolicy{
					FilterEnabled:     nil,
					ShadowEnabled:     nil,
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: "envoy.filters.http.csrf",
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: utils.MustMessageToAny(&envoycsrf.CsrfPolicy{
							FilterEnabled:     nil,
							ShadowEnabled:     nil,
							AdditionalOrigins: envoyAdditionalOrigins,
						}),
					},
				},
				Stage: plugins.FilterStage{
					RelativeTo: 8,
								Weight:     0,
							},
						},
		}))
	})

	It("copies the csrf config from the listener to the filter with filters enabled", func() {
		apiAdditionalOrigins := []*api_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &api_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		envoyAdditionalOrigins := []*envoy_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Csrf: &v31.CsrfPolicy{
					FilterEnabled: &v3.RuntimeFractionalPercent{
						DefaultValue: &api_type_v3.FractionalPercent{
							Numerator: uint32(1),
							Denominator: api_type_v3.FractionalPercent_HUNDRED,
						},
					},
					ShadowEnabled:     nil,
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: "envoy.filters.http.csrf",
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: utils.MustMessageToAny(&envoycsrf.CsrfPolicy{
							FilterEnabled: &envoy_config_core_v3.RuntimeFractionalPercent{
							DefaultValue: &envoytype.FractionalPercent{
							Numerator: uint32(1),
							Denominator: envoytype.FractionalPercent_HUNDRED,
						},
						},
							ShadowEnabled:     nil,
							AdditionalOrigins: envoyAdditionalOrigins,
						}),
					},
				},
				Stage: plugins.FilterStage{
					RelativeTo: 8,
					Weight:     0,
				},
			},
		}))
	})

	It("copies the csrf config from the listener to the filter with shadow enabled", func() {
		apiAdditionalOrigins := []*api_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &api_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		envoyAdditionalOrigins := []*envoy_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Csrf: &v31.CsrfPolicy{
					FilterEnabled: nil,
					ShadowEnabled: &v3.RuntimeFractionalPercent{
						DefaultValue: &api_type_v3.FractionalPercent{
							Numerator: uint32(1),
							Denominator: api_type_v3.FractionalPercent_HUNDRED,
						},
					},
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: "envoy.filters.http.csrf",
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: utils.MustMessageToAny(&envoycsrf.CsrfPolicy{
							FilterEnabled: nil,
							ShadowEnabled: &envoy_config_core_v3.RuntimeFractionalPercent{
								DefaultValue: &envoytype.FractionalPercent{
									Numerator: uint32(1),
									Denominator: envoytype.FractionalPercent_HUNDRED,
								},
							},
							AdditionalOrigins: envoyAdditionalOrigins,
						}),
					},
				},
				Stage: plugins.FilterStage{
					RelativeTo: 8,
					Weight:     0,
				},
			},
		}))
	})

	It("copies the csrf config from the listener to the filter with filters enabled, shadow ignored", func() {
		apiAdditionalOrigins := []*api_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &api_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		envoyAdditionalOrigins := []*envoy_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Csrf: &v31.CsrfPolicy{
					FilterEnabled: &v3.RuntimeFractionalPercent{
						DefaultValue: &api_type_v3.FractionalPercent{
							Numerator: uint32(1),
							Denominator: api_type_v3.FractionalPercent_HUNDRED,
						},
					},
					ShadowEnabled:  &v3.RuntimeFractionalPercent{
						DefaultValue: &api_type_v3.FractionalPercent{
							Numerator: uint32(1),
							Denominator: api_type_v3.FractionalPercent_HUNDRED,
						},
					},
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: "envoy.filters.http.csrf",
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: utils.MustMessageToAny(&envoycsrf.CsrfPolicy{
							FilterEnabled: &envoy_config_core_v3.RuntimeFractionalPercent{
								DefaultValue: &envoytype.FractionalPercent{
									Numerator: uint32(1),
									Denominator: envoytype.FractionalPercent_HUNDRED,
								},
							},
							ShadowEnabled: &envoy_config_core_v3.RuntimeFractionalPercent{
								DefaultValue: &envoytype.FractionalPercent{
									Numerator: uint32(1),
									Denominator: envoytype.FractionalPercent_HUNDRED,
								},
							},
							AdditionalOrigins: envoyAdditionalOrigins,
						}),
					},
				},
				Stage: plugins.FilterStage{
					RelativeTo: 8,
					Weight:     0,
				},
			},
		}))
	})

	It("allows route specific csrf config", func() {
		apiAdditionalOrigins := []*api_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &api_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		envoyAdditionalOrigins := []*envoy_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		p := NewPlugin()
		out := &envoy_config_route_v3.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				Csrf: &v31.CsrfPolicy{
					FilterEnabled:     nil,
					ShadowEnabled:     nil,
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		}, out)

		var cfg envoycsrf.CsrfPolicy
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()["envoy.filters.http.csrf"], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetAdditionalOrigins()).To(Equal(envoyAdditionalOrigins))
	})

	It("allows vhost specific csrf config", func() {
		apiAdditionalOrigins := []*api_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &api_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		envoyAdditionalOrigins := []*envoy_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		p := NewPlugin()
		out := &envoy_config_route_v3.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				Csrf: &v31.CsrfPolicy{
					FilterEnabled:     nil,
					ShadowEnabled:     nil,
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		}, out)

		var cfg envoycsrf.CsrfPolicy
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()["envoy.filters.http.csrf"], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetAdditionalOrigins()).To(Equal(envoyAdditionalOrigins))
	})

	It("allows weighted destination specific csrf config", func() {
		apiAdditionalOrigins := []*api_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &api_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		envoyAdditionalOrigins := []*envoy_type_matcher_v3.StringMatcher {
			{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		p := NewPlugin()
		out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				Csrf: &v31.CsrfPolicy{
					FilterEnabled:     nil,
					ShadowEnabled:     nil,
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		}, out)

		var cfg envoycsrf.CsrfPolicy
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()["envoy.filters.http.csrf"], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetAdditionalOrigins()).To(Equal(envoyAdditionalOrigins))
	})

})
