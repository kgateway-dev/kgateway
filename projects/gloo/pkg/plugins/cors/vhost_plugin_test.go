package cors

import (
	"strings"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
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
		plugin.Init(plugins.InitParams{})
		allowCredentials1 := true
		in1 := &v1.CorsPolicy{
			AllowOrigin:      allowOrigin1,
			AllowOriginRegex: allowOriginRegex1,
			AllowMethods:     allowMethods1,
			AllowHeaders:     allowHeaders1,
			ExposeHeaders:    exposeHeaders1,
			MaxAge:           maxAge1,
			AllowCredentials: allowCredentials1,
		}
		gloo1 = &v1.VirtualHost{
			CorsPolicy: in1,
		}

		out1 := &envoyroute.CorsPolicy{
			AllowOrigin:      allowOrigin1,
			AllowOriginRegex: allowOriginRegex1,
			AllowMethods:     strings.Join(allowMethods1, ","),
			AllowHeaders:     strings.Join(allowHeaders1, ","),
			ExposeHeaders:    strings.Join(exposeHeaders1, ","),
			MaxAge:           maxAge1,
			AllowCredentials: &types.BoolValue{Value: allowCredentials1},
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
				CorsPolicy: &v1.CorsPolicy{
					AllowOrigin: allowOrigin1,
				},
			}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, inRoute, out)
			Expect(err).NotTo(HaveOccurred())
			envoy1min := &envoyroute.VirtualHost{
				Cors: &envoyroute.CorsPolicy{
					AllowOrigin: allowOrigin1,
				},
			}
			Expect(out).To(Equal(envoy1min))
		})
		It("should process virtual hosts - empty specification", func() {
			out := &envoyroute.VirtualHost{}
			inRoute := &v1.VirtualHost{
				CorsPolicy: &v1.CorsPolicy{},
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
			gloo1null := &v1.VirtualHost{
				CorsPolicy: nil,
			}
			err := plugin.(plugins.VirtualHostPlugin).ProcessVirtualHost(params, gloo1null, out)
			Expect(err).NotTo(HaveOccurred())
			envoy1null := &envoyroute.VirtualHost{}
			Expect(out).To(Equal(envoy1null))
		})
	})
})
