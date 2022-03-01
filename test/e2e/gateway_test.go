package e2e_test

import (
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func getTrivialVirtualServiceForUpstream(ns string, upstream *core.ResourceRef) *gatewayv1.VirtualService {
	vs := getTrivialVirtualService(ns)
	vs.VirtualHost.Routes[0].GetRouteAction().GetSingle().DestinationType = &gloov1.Destination_Upstream{
		Upstream: upstream,
	}
	return vs
}

func getTrivialVirtualServiceForService(ns string, service *core.ResourceRef, port uint32) *gatewayv1.VirtualService {
	vs := getTrivialVirtualService(ns)
	vs.VirtualHost.Routes[0].GetRouteAction().GetSingle().DestinationType = &gloov1.Destination_Kube{
		Kube: &gloov1.KubernetesServiceDestination{
			Ref:  service,
			Port: port,
		},
	}
	return vs
}

func getTrivialVirtualService(ns string) *gatewayv1.VirtualService {
	return &gatewayv1.VirtualService{
		Metadata: &core.Metadata{
			Name:      "vs",
			Namespace: ns,
		},
		VirtualHost: &gatewayv1.VirtualHost{
			Domains: []string{"*"},
			Routes: []*gatewayv1.Route{{
				Action: &gatewayv1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Single{
							Single: &gloov1.Destination{},
						},
					},
				},
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/",
						},
						Headers: []*matchers.HeaderMatcher{
							{
								Name:        "this-header-must-not-be-present",
								InvertMatch: true,
							},
						},
					},
				},
			}},
		},
	}
}

// Given a proxy, reuturns the non-ssl listener from
// that proxy, or nil if it can't be found
func getNonSSLListener(proxy *gloov1.Proxy) *gloov1.Listener {
	for _, l := range proxy.Listeners {
		if l.BindPort == defaults.HttpPort {
			return l
		}
	}
	return nil
}
