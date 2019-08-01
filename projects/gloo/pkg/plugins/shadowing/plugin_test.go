package shadowing

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/shadowing"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {

	It("should work on valid inputs", func() {
		p := NewPlugin()

		upRef := &core.ResourceRef{
			Name:      "some-upstream",
			Namespace: "default",
		}
		in := &v1.Route{
			RoutePlugins: &v1.RoutePlugins{
				Shadowing: &shadowing.RouteShadowing{
					UpstreamRef: upRef,
					Percent:     100,
				},
			},
		}
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetRoute().RequestMirrorPolicy.RuntimeFraction.DefaultValue.Numerator).To(Equal(uint32(100)))
		Expect(out.GetRoute().RequestMirrorPolicy.Cluster).To(Equal("some-upstream_default"))
	})

	It("should handle empty configs", func() {
		p := NewPlugin()
		in := &v1.Route{}
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should error when set on invalid routes", func() {
		p := NewPlugin()

		upRef := &core.ResourceRef{
			Name:      "some-upstream",
			Namespace: "default",
		}
		in := &v1.Route{
			Matcher: nil,
			Action:  nil,
			RoutePlugins: &v1.RoutePlugins{
				Shadowing: &shadowing.RouteShadowing{
					UpstreamRef: upRef,
					Percent:     190,
				},
			},
		}
		// a redirect route is not a valid target for this plugin
		out := &envoyroute.Route{
			Action: &envoyroute.Route_Redirect{
				Redirect: &envoyroute.RedirectAction{},
			},
		}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(InvalidRouteActionError))

		// a direct response route is not a valid target for this plugin
		out = &envoyroute.Route{
			Action: &envoyroute.Route_DirectResponse{
				DirectResponse: &envoyroute.DirectResponseAction{},
			},
		}
		err = p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(InvalidRouteActionError))
	})

})
