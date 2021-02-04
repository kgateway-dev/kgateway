package basicroute_test

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"

	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/retries"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/basicroute"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ = Describe("prefix rewrite", func() {
	It("works", func() {
		p := NewPlugin()
		routeAction := &envoy_config_route_v3.RouteAction{
			PrefixRewrite: "/",
		}
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: routeAction,
			},
		}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				PrefixRewrite: &wrappers.StringValue{Value: "/foo"},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.PrefixRewrite).To(Equal("/foo"))
	})

	It("distinguishes between empty string and nil", func() {
		p := NewPlugin()
		routeAction := &envoy_config_route_v3.RouteAction{
			PrefixRewrite: "/",
		}
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
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
				PrefixRewrite: &wrappers.StringValue{Value: ""},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.PrefixRewrite).To(BeEmpty())
	})
})

var _ = Describe("regex rewrite", func() {
	It("works, config alone supplies max program size", func() {
		p := NewPlugin()
		routeAction := &envoy_config_route_v3.RouteAction{
			RegexRewrite: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
				Pattern: &envoy_type_matcher_v3.RegexMatcher{
					Regex: "/",
					EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
						GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
					},
				},
				Substitution: "/bar",
			},
		}
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: routeAction,
			},
		}

		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				RegexRewrite: &v3.RegexMatchAndSubstitute{
					Pattern: &v3.RegexMatcher{
						Regex: "/",
						EngineType: &v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &v3.RegexMatcher_GoogleRE2{
								MaxProgramSize: &wrappers.UInt32Value{Value: 1024},
							},
						},
					},
					Substitution: "/foo",
				},
			},
		}, out)

		rmas := &envoy_type_matcher_v3.RegexMatchAndSubstitute{
			Pattern: &envoy_type_matcher_v3.RegexMatcher{
				Regex: "/",
				EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
					GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{
						MaxProgramSize: &wrappers.UInt32Value{Value: 1024},
					},
				},
			},
			Substitution: "/foo",
		}

		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.RegexRewrite).To(Equal(rmas))
	})
	It("works, ctx alone supplies max program size", func() {
		p := NewPlugin()
		routeAction := &envoy_config_route_v3.RouteAction{
			RegexRewrite: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
				Pattern: &envoy_type_matcher_v3.RegexMatcher{
					Regex: "/",
					EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
						GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
					},
				},
				Substitution: "/bar",
			},
		}
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: routeAction,
			},
		}

		rps := plugins.RouteParams{}
		rps.Ctx = settingsutil.WithSettings(context.Background(), &v1.Settings{
			Gloo: &v1.GlooOptions{
				RegexMaxProgramSize: &wrappers.UInt32Value{Value: 256},
			},
		})

		err := p.ProcessRoute(rps, &v1.Route{
			Options: &v1.RouteOptions{
				RegexRewrite: &v3.RegexMatchAndSubstitute{
					Pattern: &v3.RegexMatcher{
						Regex: "/",
						EngineType: &v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &v3.RegexMatcher_GoogleRE2{},
						},
					},
					Substitution: "/foo",
				},
			},
		}, out)

		rmas := &envoy_type_matcher_v3.RegexMatchAndSubstitute{
			Pattern: &envoy_type_matcher_v3.RegexMatcher{
				Regex: "/",
				EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
					GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{
						MaxProgramSize: &wrappers.UInt32Value{Value: 256},
					},
				},
			},
			Substitution: "/foo",
		}

		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.RegexRewrite).To(Equal(rmas))
	})
	It("works, ctx max program size more restrictive", func() {
		p := NewPlugin()
		routeAction := &envoy_config_route_v3.RouteAction{
			RegexRewrite: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
				Pattern: &envoy_type_matcher_v3.RegexMatcher{
					Regex: "/",
					EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
						GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
					},
				},
				Substitution: "/bar",
			},
		}
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: routeAction,
			},
		}

		rps := plugins.RouteParams{}
		rps.Ctx = settingsutil.WithSettings(context.Background(), &v1.Settings{
			Gloo: &v1.GlooOptions{
				RegexMaxProgramSize: &wrappers.UInt32Value{Value: 256},
			},
		})

		err := p.ProcessRoute(rps, &v1.Route{
			Options: &v1.RouteOptions{
				RegexRewrite: &v3.RegexMatchAndSubstitute{
					Pattern: &v3.RegexMatcher{
						Regex: "/",
						EngineType: &v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &v3.RegexMatcher_GoogleRE2{
								MaxProgramSize: &wrappers.UInt32Value{Value: 1024},
							},
						},
					},
					Substitution: "/foo",
				},
			},
		}, out)

		rmas := &envoy_type_matcher_v3.RegexMatchAndSubstitute{
			Pattern: &envoy_type_matcher_v3.RegexMatcher{
				Regex: "/",
				EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
					GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{
						MaxProgramSize: &wrappers.UInt32Value{Value: 256},
					},
				},
			},
			Substitution: "/foo",
		}

		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.RegexRewrite).To(Equal(rmas))
	})
})

var _ = Describe("timeout", func() {
	It("works", func() {
		t := prototime.DurationToProto(time.Minute)
		p := NewPlugin()
		routeAction := &envoy_config_route_v3.RouteAction{}
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: routeAction,
			},
		}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				Timeout: t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.Timeout).NotTo(BeNil())
		Expect(routeAction.Timeout).To(Equal(t))
	})
})

var _ = Describe("retries", func() {

	var (
		plugin              *Plugin
		retryPolicy         *retries.RetryPolicy
		expectedRetryPolicy *envoy_config_route_v3.RetryPolicy
	)
	BeforeEach(func() {
		t := prototime.DurationToProto(time.Minute)
		retryPolicy = &retries.RetryPolicy{
			RetryOn:       "if at first you don't succeed",
			NumRetries:    5,
			PerTryTimeout: t,
		}
		expectedRetryPolicy = &envoy_config_route_v3.RetryPolicy{
			RetryOn: "if at first you don't succeed",
			NumRetries: &wrappers.UInt32Value{
				Value: 5,
			},
			PerTryTimeout: t,
		}

		plugin = NewPlugin()
	})

	It("works", func() {
		routeAction := &envoy_config_route_v3.RouteAction{}
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
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
		out := &envoy_config_route_v3.VirtualHost{}
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
		routeAction := &envoy_config_route_v3.RouteAction{
			HostRewriteSpecifier: &envoy_config_route_v3.RouteAction_HostRewriteLiteral{HostRewriteLiteral: "/"},
		}
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: routeAction,
			},
		}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HostRewriteType: &v1.RouteOptions_HostRewrite{HostRewrite: "/foo"},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.GetHostRewriteLiteral()).To(Equal("/foo"))
	})

	It("distinguishes between empty string and nil", func() {
		p := NewPlugin()
		routeAction := &envoy_config_route_v3.RouteAction{
			HostRewriteSpecifier: &envoy_config_route_v3.RouteAction_HostRewriteLiteral{HostRewriteLiteral: "/"},
		}
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: routeAction,
			},
		}

		// should be no-op
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.GetHostRewriteLiteral()).To(Equal("/"))

		// should rewrite host rewrite
		err = p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HostRewriteType: &v1.RouteOptions_HostRewrite{HostRewrite: ""},
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.GetHostRewriteLiteral()).To(BeEmpty())
	})

	It("sets auto_host_rewrite", func() {

		p := NewPlugin()
		routeAction := &envoy_config_route_v3.RouteAction{
			HostRewriteSpecifier: &envoy_config_route_v3.RouteAction_AutoHostRewrite{
				AutoHostRewrite: &wrappers.BoolValue{
					Value: false,
				},
			},
		}
		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: routeAction,
			},
		}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HostRewriteType: &v1.RouteOptions_AutoHostRewrite{
					AutoHostRewrite: &wrappers.BoolValue{
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

		routeAction := &envoy_config_route_v3.RouteAction{}

		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: routeAction,
			},
		}

		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				Upgrades: []*protocol_upgrade.ProtocolUpgradeConfig{
					{
						UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
							Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
								Enabled: &wrappers.BoolValue{Value: true},
							},
						},
					},
				},
			},
		}, out)

		Expect(err).NotTo(HaveOccurred())
		Expect(len(routeAction.GetUpgradeConfigs())).To(Equal(1))
		Expect(routeAction.GetUpgradeConfigs()[0].UpgradeType).To(Equal("websocket"))
		Expect(routeAction.GetUpgradeConfigs()[0].Enabled.Value).To(Equal(true))
	})
	It("fails on double config", func() {
		p := NewPlugin()

		routeAction := &envoy_config_route_v3.RouteAction{}

		out := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: routeAction,
			},
		}

		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				Upgrades: []*protocol_upgrade.ProtocolUpgradeConfig{
					{
						UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
							Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
								Enabled: &wrappers.BoolValue{Value: true},
							},
						},
					},
					{
						UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
							Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
								Enabled: &wrappers.BoolValue{Value: true},
							},
						},
					},
				},
			},
		}, out)

		Expect(err).To(MatchError(ContainSubstring("upgrade config websocket is not unique")))
	})
})
