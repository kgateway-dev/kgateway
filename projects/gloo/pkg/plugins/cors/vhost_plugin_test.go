package cors

import (
	"strings"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_config_cors_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("VirtualHost Plugin", func() {
	var (
		params plugins.VirtualHostParams
		plugin plugins.Plugin
		gloo1  *v1.VirtualHost
		envoy1 *envoy_config_route_v3.VirtualHost

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
		plugin.Init(plugins.InitParams{})
		allowCredentials1 := true
		in1 := &cors.CorsPolicy{
			AllowOrigin:      allowOrigin1,
			AllowOriginRegex: allowOriginRegex1,
			AllowMethods:     allowMethods1,
			AllowHeaders:     allowHeaders1,
			ExposeHeaders:    exposeHeaders1,
			MaxAge:           maxAge1,
			AllowCredentials: allowCredentials1,
		}
		gloo1 = &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				Cors: in1,
			},
		}

		out1 := &envoy_config_cors_v3.CorsPolicy{

			AllowOriginStringMatch: []*envoy_type_matcher_v3.StringMatcher{
				{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{Exact: allowOrigin1[0]},
				},
				{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{Exact: allowOrigin1[1]},
				},
				{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
						SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
							EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{}},
							Regex:      allowOriginRegex1[0],
						},
					},
				},
				{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
						SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
							EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{}},
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
		}
		typedConfig, err := utils.MessageToAny(out1)
		Expect(err).NotTo(HaveOccurred())
		envoy1 = &envoy_config_route_v3.VirtualHost{
			TypedPerFilterConfig: map[string]*any.Any{
				"envoy.filters.http.cors": typedConfig,
			},
		}

		params = plugins.VirtualHostParams{}

	})

	Context("CORS", func() {
		It("should process virtual hosts - full specification", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, gloo1, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal(envoy1))
		})
		It("should process virtual hosts - minimal specification", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			inRoute := &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					Cors: &cors.CorsPolicy{
						AllowOrigin: allowOrigin1,
					},
				},
			}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, inRoute, out)
			Expect(err).NotTo(HaveOccurred())
			out1min := &envoy_config_cors_v3.CorsPolicy{
				AllowOriginStringMatch: []*envoy_type_matcher_v3.StringMatcher{
					{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{Exact: allowOrigin1[0]},
					},
					{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{Exact: allowOrigin1[1]},
					},
				},
			}
			typedConfig, err := utils.MessageToAny(out1min)
			Expect(err).NotTo(HaveOccurred())
			envoy1min := &envoy_config_route_v3.VirtualHost{
				TypedPerFilterConfig: map[string]*any.Any{
					"envoy.filters.http.cors": typedConfig,
				},
			}
			Expect(out).To(Equal(envoy1min))
		})
		It("should process virtual hosts - empty specification", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			inRoute := &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					Cors: &cors.CorsPolicy{},
				},
			}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, inRoute, out)
			Expect(err).To(HaveOccurred())

			envoy1empty := &envoy_config_route_v3.VirtualHost{}
			Expect(out).To(Equal(envoy1empty))
		})
		It("should process virtual hosts - ignore route filter disabled spec", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			inRoute := &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					Cors: &cors.CorsPolicy{
						DisableForRoute: true,
					},
				},
			}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, inRoute, out)
			Expect(err).To(HaveOccurred())
			envoy1empty := &envoy_config_route_v3.VirtualHost{}
			Expect(out).To(Equal(envoy1empty))
		})
		It("should process virtual hosts - null specification", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			gloo1null := &v1.VirtualHost{}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, gloo1null, out)
			Expect(err).NotTo(HaveOccurred())
			envoy1null := &envoy_config_route_v3.VirtualHost{}
			Expect(out).To(Equal(envoy1null))
		})
	})
})
