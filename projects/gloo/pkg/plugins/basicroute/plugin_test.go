package basicroute_test

import (
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/upgrade"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/retries"
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
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				PrefixRewrite: &types.StringValue{Value: "/foo"},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.PrefixRewrite).To(Equal("/foo"))
	})

	It("distinguishes between empty string and nil", func() {
		p := NewPlugin()
		routeAction := &envoyroute.RouteAction{
			PrefixRewrite: "/",
		}
		out := &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: routeAction,
			},
		}

		// should be no-op
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.PrefixRewrite).To(Equal("/"))

		// should rewrite prefix rewrite
		err = p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				PrefixRewrite: &types.StringValue{Value: ""},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.PrefixRewrite).To(BeEmpty())
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
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				Timeout: &t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.Timeout).NotTo(BeNil())
		Expect(*routeAction.Timeout).To(Equal(t))
	})
})

var _ = Describe("retries", func() {

	var (
		plugin              *Plugin
		retryPolicy         *retries.RetryPolicy
		expectedRetryPolicy *envoyroute.RetryPolicy
	)
	BeforeEach(func() {
		t := time.Minute
		retryPolicy = &retries.RetryPolicy{
			RetryOn:       "if at first you don't succeed",
			NumRetries:    5,
			PerTryTimeout: &t,
		}
		expectedRetryPolicy = &envoyroute.RetryPolicy{
			RetryOn: "if at first you don't succeed",
			NumRetries: &types.UInt32Value{
				Value: 5,
			},
			PerTryTimeout: &t,
		}

		plugin = NewPlugin()
	})

	It("works", func() {
		routeAction := &envoyroute.RouteAction{}
		out := &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: routeAction,
			},
		}
		err := plugin.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				Retries: retryPolicy,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.RetryPolicy).To(Equal(expectedRetryPolicy))
	})
	It("works on vhost", func() {
		out := &envoyroute.VirtualHost{}
		err := plugin.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				Retries: retryPolicy,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RetryPolicy).To(Equal(expectedRetryPolicy))
	})
})

var _ = Describe("host rewrite", func() {
	It("rewrites using provided string", func() {

		p := NewPlugin()
		routeAction := &envoyroute.RouteAction{
			HostRewriteSpecifier: &envoyroute.RouteAction_HostRewrite{HostRewrite: "/"},
		}
		out := &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: routeAction,
			},
		}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HostRewriteType: &v1.RouteOptions_HostRewrite{HostRewrite: "/foo"},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.GetHostRewrite()).To(Equal("/foo"))
	})

	It("distinguishes between empty string and nil", func() {
		p := NewPlugin()
		routeAction := &envoyroute.RouteAction{
			HostRewriteSpecifier: &envoyroute.RouteAction_HostRewrite{HostRewrite: "/"},
		}
		out := &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: routeAction,
			},
		}

		// should be no-op
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.GetHostRewrite()).To(Equal("/"))

		// should rewrite host rewrite
		err = p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HostRewriteType: &v1.RouteOptions_HostRewrite{HostRewrite: ""},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.GetHostRewrite()).To(BeEmpty())
	})

	It("sets auto_host_rewrite", func() {

		p := NewPlugin()
		routeAction := &envoyroute.RouteAction{
			HostRewriteSpecifier: &envoyroute.RouteAction_AutoHostRewrite{
				AutoHostRewrite: &types.BoolValue{
					Value: false,
				},
			},
		}
		out := &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: routeAction,
			},
		}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HostRewriteType: &v1.RouteOptions_AutoHostRewrite{
					AutoHostRewrite: &types.BoolValue{
						Value: true,
					},
				},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.GetAutoHostRewrite().GetValue()).To(Equal(true))
	})
})

var _ = Describe("upgrades", func() {
	It("works", func() {
		p := NewPlugin()

		routeAction := &envoyroute.RouteAction{}

		out := &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: routeAction,
			},
		}

		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				UpgradeConfigs: []*upgrade.UpgradeConfig{
					{
						UpgradeType: &upgrade.UpgradeConfig_Websocket{},
					},
				},
			},
		}, out)

		Expect(err).NotTo(HaveOccurred())
		Expect(len(routeAction.GetUpgradeConfigs())).To(Equal(1))
		Expect(routeAction.GetUpgradeConfigs()[0].UpgradeType).To(Equal("websocket"))
	})
})
