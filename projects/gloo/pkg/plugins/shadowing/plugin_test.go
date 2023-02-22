package shadowing

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/shadowing"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {

	It("should work on valid inputs, with uninitialized outputs", func() {
		p := NewPlugin()

		upRef := &core.ResourceRef{
			Name:      "some-upstream",
			Namespace: "default",
		}
		in := &v1.Route{
			Options: &v1.RouteOptions{
				Shadowing: &shadowing.RouteShadowing{
					Upstream:   upRef,
					Percentage: 100,
				},
			},
		}
		out := &envoy_config_route_v3.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).NotTo(HaveOccurred())
		checkFraction(out.GetRoute().GetRequestMirrorPolicies()[0].GetRuntimeFraction(), 100)
		Expect(out.GetRoute().GetRequestMirrorPolicies()[0].GetCluster()).To(Equal("some-upstream_default"))
	})

	It("should work on valid inputs, with initialized outputs", func() {
		p := NewPlugin()

		upRef := &core.ResourceRef{
			Name:      "some-upstream",
			Namespace: "default",
		}
		in := &v1.Route{
			Options: &v1.RouteOptions{
				Shadowing: &shadowing.RouteShadowing{
					Upstream:   upRef,
					Percentage: 100,
				},
			},
		}
		var out = &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					PrefixRewrite: "/something/set/by/another/plugin",
				},
			},
		}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).NotTo(HaveOccurred())
		checkFraction(out.GetRoute().GetRequestMirrorPolicies()[0].RuntimeFraction, 100)
		Expect(out.GetRoute().GetRequestMirrorPolicies()[0].Cluster).To(Equal("some-upstream_default"))
		Expect(out.GetRoute().PrefixRewrite).To(Equal("/something/set/by/another/plugin"))
	})

	It("should not error on empty configs", func() {
		p := NewPlugin()
		in := &v1.Route{}
		out := &envoy_config_route_v3.Route{}
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
			Options: &v1.RouteOptions{
				Shadowing: &shadowing.RouteShadowing{
					Upstream:   upRef,
					Percentage: 100,
				},
			},
		}
		// a redirect route is not a valid target for this plugin
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Redirect{
				Redirect: &envoy_config_route_v3.RedirectAction{},
			},
		}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).To(HaveOccurred())
		Expect(err).To(HaveInErrorChain(InvalidRouteActionError))

		// a direct response route is not a valid target for this plugin
		out = &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_DirectResponse{
				DirectResponse: &envoy_config_route_v3.DirectResponseAction{},
			},
		}
		err = p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).To(HaveOccurred())
		Expect(err).To(HaveInErrorChain(InvalidRouteActionError))
	})

	It("should error when given invalid specs", func() {
		p := NewPlugin()

		upRef := &core.ResourceRef{
			Name:      "some-upstream",
			Namespace: "default",
		}
		in := &v1.Route{
			Options: &v1.RouteOptions{
				Shadowing: &shadowing.RouteShadowing{
					Upstream:   upRef,
					Percentage: 200,
				},
			},
		}
		out := &envoy_config_route_v3.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).To(HaveOccurred())
		Expect(err).To(HaveInErrorChain(InvalidNumeratorError(200)))

		in = &v1.Route{
			Options: &v1.RouteOptions{
				Shadowing: &shadowing.RouteShadowing{
					Percentage: 100,
				},
			},
		}
		out = &envoy_config_route_v3.Route{}
		err = p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).To(HaveOccurred())
		Expect(err).To(HaveInErrorChain(UnspecifiedUpstreamError))
	})

})

func checkFraction(frac *envoy_config_core_v3.RuntimeFractionalPercent, percentage float32) {
	Expect(frac.DefaultValue.Numerator).To(Equal(uint32(percentage * 10000)))
}
