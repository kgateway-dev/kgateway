package csrf_test

import (
	"context"

	envoy_config_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycsrf "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_type_matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	gloo_config_core "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	gloocsrf "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	gloo_type_matcher "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	glootype "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/csrf"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/golang/protobuf/ptypes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("plugin", func() {

	var apiFilter gloo_config_core.RuntimeFractionalPercent
	var envoyFilter envoy_config_core.RuntimeFractionalPercent
	var apiAdditionalOrigins []*gloo_type_matcher.StringMatcher
	var envoyAdditionalOrigins []*envoy_type_matcher.StringMatcher

	BeforeEach(func() {
		apiFilter = gloo_config_core.RuntimeFractionalPercent{
			DefaultValue: &glootype.FractionalPercent{
				Numerator:   uint32(100),
				Denominator: glootype.FractionalPercent_HUNDRED,
			},
		}

		envoyFilter = envoy_config_core.RuntimeFractionalPercent{
			DefaultValue: &envoytype.FractionalPercent{
				Numerator:   uint32(100),
				Denominator: envoytype.FractionalPercent_HUNDRED,
			},
		}

		apiAdditionalOrigins = []*gloo_type_matcher.StringMatcher{
			{
				MatchPattern: &gloo_type_matcher.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		envoyAdditionalOrigins = []*envoy_type_matcher.StringMatcher{
			{
				MatchPattern: &envoy_type_matcher.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}
	})

	It("copies the csrf config from the listener to the filter with filters enabled", func() {
		np := NewPlugin()
		np.Init(plugins.InitParams{Ctx: context.TODO(), Settings: &v1.Settings{Gloo: &v1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false}}}})
		filters, err := np.HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Csrf: &gloocsrf.CsrfPolicy{
					FilterEnabled:     &apiFilter,
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		typedConfig, err := utils.MessageToAny(&envoycsrf.CsrfPolicy{
			FilterEnabled:     &envoyFilter,
			AdditionalOrigins: envoyAdditionalOrigins,
		})
		Expect(err).NotTo(HaveOccurred())
		expectedStageFilter := plugins.StagedHttpFilter{
			HttpFilter: &envoyhcm.HttpFilter{
				Name: FilterName,
				ConfigType: &envoyhcm.HttpFilter_TypedConfig{
					TypedConfig: typedConfig,
				},
			},
			Stage: plugins.FilterStage{
				RelativeTo: 8,
				Weight:     0,
			},
		}

		Expect(filters[0].HttpFilter).To(matchers.MatchProto(expectedStageFilter.HttpFilter))
		Expect(filters[0].Stage).To(Equal(expectedStageFilter.Stage))
	})

	It("copies the csrf config from the listener to the filter with shadow enabled", func() {
		np := NewPlugin()
		np.Init(plugins.InitParams{Ctx: context.TODO(), Settings: &v1.Settings{Gloo: &v1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false}}}})
		filters, err := np.HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Csrf: &gloocsrf.CsrfPolicy{
					ShadowEnabled:     &apiFilter,
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		typedConfig, err := utils.MessageToAny(&envoycsrf.CsrfPolicy{
			FilterEnabled: &envoy_config_core.RuntimeFractionalPercent{
				DefaultValue: &envoytype.FractionalPercent{},
			},
			ShadowEnabled:     &envoyFilter,
			AdditionalOrigins: envoyAdditionalOrigins,
		})
		Expect(err).NotTo(HaveOccurred())
		expectedStageFilter := plugins.StagedHttpFilter{
			HttpFilter: &envoyhcm.HttpFilter{
				Name: FilterName,
				ConfigType: &envoyhcm.HttpFilter_TypedConfig{
					TypedConfig: typedConfig,
				},
			},
			Stage: plugins.FilterStage{
				RelativeTo: 8,
				Weight:     0,
			},
		}

		Expect(filters[0].HttpFilter).To(matchers.MatchProto(expectedStageFilter.HttpFilter))
		Expect(filters[0].Stage).To(Equal(expectedStageFilter.Stage))
	})

	It("copies the csrf config from the listener to the filter with both enabled and shadow mode fields", func() {
		np := NewPlugin()
		np.Init(plugins.InitParams{Ctx: context.TODO(), Settings: &v1.Settings{Gloo: &v1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false}}}})

		filters, err := np.HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Csrf: &gloocsrf.CsrfPolicy{
					FilterEnabled:     &apiFilter,
					ShadowEnabled:     &apiFilter,
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		typedConfig, err := utils.MessageToAny(&envoycsrf.CsrfPolicy{
			FilterEnabled:     &envoyFilter,
			ShadowEnabled:     &envoyFilter,
			AdditionalOrigins: envoyAdditionalOrigins,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: FilterName,
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: typedConfig,
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
		p := NewPlugin()
		p.Init(plugins.InitParams{Ctx: context.TODO(), Settings: &v1.Settings{Gloo: &v1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false}}}})

		out := &envoy_config_route.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				Csrf: &gloocsrf.CsrfPolicy{
					FilterEnabled:     &apiFilter,
					ShadowEnabled:     &gloo_config_core.RuntimeFractionalPercent{},
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		}, out)

		var cfg envoycsrf.CsrfPolicy
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()[FilterName], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetAdditionalOrigins()).To(Equal(envoyAdditionalOrigins))
		Expect(cfg.GetFilterEnabled()).To(Equal(&envoyFilter))
	})

	It("allows vhost specific csrf config", func() {
		p := NewPlugin()
		p.Init(plugins.InitParams{Ctx: context.TODO(), Settings: &v1.Settings{Gloo: &v1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false}}}})

		out := &envoy_config_route.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				Csrf: &gloocsrf.CsrfPolicy{
					FilterEnabled:     &apiFilter,
					ShadowEnabled:     &gloo_config_core.RuntimeFractionalPercent{},
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		}, out)

		var cfg envoycsrf.CsrfPolicy
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()[FilterName], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetAdditionalOrigins()).To(Equal(envoyAdditionalOrigins))
		Expect(cfg.GetFilterEnabled()).To(Equal(&envoyFilter))
	})

	It("allows weighted destination specific csrf config", func() {
		p := NewPlugin()
		p.Init(plugins.InitParams{Ctx: context.TODO(), Settings: &v1.Settings{Gloo: &v1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false}}}})

		out := &envoy_config_route.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				Csrf: &gloocsrf.CsrfPolicy{
					FilterEnabled:     &apiFilter,
					ShadowEnabled:     &gloo_config_core.RuntimeFractionalPercent{},
					AdditionalOrigins: apiAdditionalOrigins,
				},
			},
		}, out)

		var cfg envoycsrf.CsrfPolicy
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()[FilterName], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetAdditionalOrigins()).To(Equal(envoyAdditionalOrigins))
		Expect(cfg.GetFilterEnabled()).To(Equal(&envoyFilter))
	})

})
