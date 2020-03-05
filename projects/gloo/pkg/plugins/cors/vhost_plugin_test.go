package cors

import (
	"strings"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("VirtualHost Plugin", func() {
	var (
		params plugins.VirtualHostParams
		plugin plugins.Plugin
		gloo1  *v1.VirtualHost
		envoy1 *envoyroute.VirtualHost

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

		out1 := &envoyroute.CorsPolicy{

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
		}
		envoy1 = &envoyroute.VirtualHost{
			Cors: out1,
		}

		params = plugins.VirtualHostParams{}

	})

	Context("CORS", func() {
		It("should process virtual hosts - full specification", func() {
			out := &envoyroute.VirtualHost{}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, gloo1, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal(envoy1))
		})
		It("should process virtual hosts - minimal specification", func() {
			out := &envoyroute.VirtualHost{}
			inRoute := &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					Cors: &cors.CorsPolicy{
						AllowOrigin: allowOrigin1,
					},
				},
			}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, inRoute, out)
			Expect(err).NotTo(HaveOccurred())
			envoy1min := &envoyroute.VirtualHost{
				Cors: &envoyroute.CorsPolicy{
					AllowOriginStringMatch: []*envoymatcher.StringMatcher{
						&envoymatcher.StringMatcher{
							MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: allowOrigin1[0]},
						},
						&envoymatcher.StringMatcher{
							MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: allowOrigin1[1]},
						},
					},
				},
			}
			Expect(out).To(Equal(envoy1min))
		})
		It("should process virtual hosts - empty specification", func() {
			out := &envoyroute.VirtualHost{}
			inRoute := &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					Cors: &cors.CorsPolicy{},
				},
			}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, inRoute, out)
			Expect(err).To(HaveOccurred())
			envoy1empty := &envoyroute.VirtualHost{
				Cors: &envoyroute.CorsPolicy{},
			}
			Expect(out).To(Equal(envoy1empty))
		})
		It("should process virtual hosts - ignore route filter disabled spec", func() {
			out := &envoyroute.VirtualHost{}
			inRoute := &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					Cors: &cors.CorsPolicy{
						DisableForRoute: true,
					},
				},
			}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, inRoute, out)
			Expect(err).To(HaveOccurred())
			envoy1empty := &envoyroute.VirtualHost{
				Cors: &envoyroute.CorsPolicy{},
			}
			Expect(out).To(Equal(envoy1empty))
		})
		It("should process virtual hosts - null specification", func() {
			out := &envoyroute.VirtualHost{}
			gloo1null := &v1.VirtualHost{}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, gloo1null, out)
			Expect(err).NotTo(HaveOccurred())
			envoy1null := &envoyroute.VirtualHost{}
			Expect(out).To(Equal(envoy1null))
		})
	})
})
