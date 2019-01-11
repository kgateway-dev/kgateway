package basicroute_test

import (
	"time"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/retries"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/basicroute"
)

var _ = Describe("prefix rewrite", func() {
	It("works", func() {

		p := NewPlugin()
		routeAction := &envoyroute.RouteAction{
			PrefixRewrite: "/",
		}
		out := &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: routeAction,
			},
		}
		err := p.ProcessRoute(plugins.Params{}, &v1.Route{
			RoutePlugins: &v1.RoutePlugins{
				PrefixRewrite: &transformation.PrefixRewrite{
					PrefixRewrite: "/foo",
				},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.PrefixRewrite).To(Equal("/foo"))
	})
})

var _ = Describe("timeout", func() {
	It("works", func() {
		t := time.Minute
		p := NewPlugin()
		routeAction := &envoyroute.RouteAction{}
		out := &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: routeAction,
			},
		}
		err := p.ProcessRoute(plugins.Params{}, &v1.Route{
			RoutePlugins: &v1.RoutePlugins{
				Timeout: &t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.Timeout).NotTo(BeNil())
		Expect(*routeAction.Timeout).To(Equal(t))
	})
})

var _ = Describe("retries", func() {
	It("works", func() {
		t := time.Minute
		p := NewPlugin()
		routeAction := &envoyroute.RouteAction{}
		out := &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: routeAction,
			},
		}
		err := p.ProcessRoute(plugins.Params{}, &v1.Route{
			RoutePlugins: &v1.RoutePlugins{
				Retries: &retries.RetryPolicy{
					RetryOn:       "if at first you don't succeed",
					NumRetries:    5,
					PerTryTimeout: &t,
				},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.RetryPolicy).NotTo(BeNil())
		Expect(*routeAction.RetryPolicy).To(Equal(envoyroute.RouteAction_RetryPolicy{
			RetryOn: "if at first you don't succeed",
			NumRetries: &types.UInt32Value{
				Value: 5,
			},
			PerTryTimeout: &t,
		}))
	})
})
