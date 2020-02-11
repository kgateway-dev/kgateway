package cors

import (
	"strings"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	"github.com/golang/protobuf/ptypes/wrappers"

	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Route Plugin", func() {
	var (
		params plugins.RouteParams
		plugin plugins.Plugin

		// values used in first example
		allowOrigin1      = []string{"solo.io", "github.com"}
		allowOriginRegex1 = []string{`*\.solo\.io`, `git.*\.com`}
		allowMethods1     = []string{"GET", "POST"}
		allowHeaders1     = []string{"allowH1", "allow2"}
		exposeHeaders1    = []string{"exHeader", "eh2"}
		maxAge1           = "5555"
	)

	BeforeEach(func() {
		plugin = NewPlugin()
		err := plugin.Init(plugins.InitParams{})
		Expect(err).NotTo(HaveOccurred())
		params = plugins.RouteParams{}

	})

	Context("CORS", func() {
		It("should full specification", func() {
			allowCredentials1 := true
			inRoute := routeWithCors(&cors.CorsPolicy{
				AllowOrigin:      allowOrigin1,
				AllowOriginRegex: allowOriginRegex1,
				AllowMethods:     allowMethods1,
				AllowHeaders:     allowHeaders1,
				ExposeHeaders:    exposeHeaders1,
				MaxAge:           maxAge1,
				AllowCredentials: allowCredentials1,
				DisableForRoute:  true,
			})
			outRoute := &envoyroute.Route{
				Action: &envoyroute.Route_Route{
					Route: &envoyroute.RouteAction{},
				},
			}
			expected := &envoyroute.CorsPolicy{
				AllowOriginStringMatch: []*envoymatcher.StringMatcher{
					&envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: allowOrigin1[0]},
					},
					&envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: allowOrigin1[1]},
					},
					&envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_SafeRegex{
							SafeRegex: &envoymatcher.RegexMatcher{
								EngineType: &envoymatcher.RegexMatcher_GoogleRe2{},
								Regex:      allowOriginRegex1[0],
							},
						},
					},
					&envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_SafeRegex{
							SafeRegex: &envoymatcher.RegexMatcher{
								EngineType: &envoymatcher.RegexMatcher_GoogleRe2{},
								Regex:      allowOriginRegex1[1],
							},
						},
					},
				},
				AllowMethods:     strings.Join(allowMethods1, ","),
				AllowHeaders:     strings.Join(allowHeaders1, ","),
				ExposeHeaders:    strings.Join(exposeHeaders1, ","),
				MaxAge:           maxAge1,
				AllowCredentials: &wrappers.BoolValue{Value: allowCredentials1},
				EnabledSpecifier: &envoyroute.CorsPolicy_FilterEnabled{
					FilterEnabled: &envoycore.RuntimeFractionalPercent{
						DefaultValue: &envoy_type.FractionalPercent{
							Numerator:   0,
							Denominator: envoy_type.FractionalPercent_HUNDRED,
						},
						RuntimeKey: runtimeKey,
					},
				},
			}

			err := plugin.(plugins.RoutePlugin).ProcessRoute(params, inRoute, outRoute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outRoute.Action.(*envoyroute.Route_Route).Route.Cors).To(Equal(expected))
		})
		It("should process  minimal specification", func() {
			inRoute := routeWithCors(&cors.CorsPolicy{
				AllowOrigin: allowOrigin1,
			})
			outRoute := basicEnvoyRoute()
			err := plugin.(plugins.RoutePlugin).ProcessRoute(params, inRoute, outRoute)
			Expect(err).NotTo(HaveOccurred())
			cSpec := &envoyroute.CorsPolicy{
				AllowOriginStringMatch: []*envoymatcher.StringMatcher{
					&envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: allowOrigin1[0]},
					},
					&envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: allowOrigin1[1]},
					},
				},
			}
			expected := basicEnvoyRouteWithCors(cSpec)
			Expect(outRoute.Action.(*envoyroute.Route_Route).Route.Cors).To(Equal(cSpec))
			Expect(outRoute).To(Equal(expected))
		})
		It("should process empty specification", func() {
			inRoute := routeWithCors(&cors.CorsPolicy{})
			outRoute := basicEnvoyRoute()
			err := plugin.(plugins.RoutePlugin).ProcessRoute(params, inRoute, outRoute)
			Expect(err).To(HaveOccurred())
			cSpec := &envoyroute.CorsPolicy{}
			expected := basicEnvoyRouteWithCors(cSpec)
			Expect(outRoute.Action.(*envoyroute.Route_Route).Route.Cors).To(Equal(cSpec))
			Expect(outRoute.String()).To(Equal(expected.String()))
			Expect(outRoute).To(Equal(expected))
		})
		It("should process null specification", func() {
			inRoute := routeWithCors(nil)
			outRoute := basicEnvoyRoute()
			err := plugin.(plugins.RoutePlugin).ProcessRoute(params, inRoute, outRoute)
			Expect(err).NotTo(HaveOccurred())
			expected := basicEnvoyRoute()
			Expect(outRoute).To(Equal(expected))
		})
	})

})

func routeWithoutCors() *v1.Route {
	return &v1.Route{
		Action: &v1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{
					Single: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &core.ResourceRef{
								Name:      "test",
								Namespace: "default",
							},
						},
					},
				},
			},
		},
	}
}

func routeWithCors(cSpec *cors.CorsPolicy) *v1.Route {
	route := routeWithoutCors()
	route.Options = &v1.RouteOptions{
		Cors: cSpec,
	}
	return route
}

func basicEnvoyRoute() *envoyroute.Route {
	return &envoyroute.Route{
		Action: &envoyroute.Route_Route{
			Route: &envoyroute.RouteAction{},
		},
	}
}

func basicEnvoyRouteWithCors(cSpec *envoyroute.CorsPolicy) *envoyroute.Route {
	return &envoyroute.Route{
		Action: &envoyroute.Route_Route{
			Route: &envoyroute.RouteAction{
				Cors: cSpec,
			},
		},
	}
}
