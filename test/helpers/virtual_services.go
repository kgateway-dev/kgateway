package helpers

import (
	"errors"

	"github.com/golang/protobuf/proto"
	"github.com/onsi/ginkgo/v2"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// VirtualServiceBuilder simplifies the process of generating VirtualServices in tests
type VirtualServiceBuilder struct {
	name      string
	namespace string

	domains            []string
	virtualHostOptions *gloov1.VirtualHostOptions
	routesByName       map[string]*v1.Route
	sslConfig          *ssl.SslConfig
}

func BuilderFromVirtualService(vs *v1.VirtualService) *VirtualServiceBuilder {
	builder := &VirtualServiceBuilder{
		name:               vs.GetMetadata().GetName(),
		namespace:          vs.GetMetadata().GetNamespace(),
		domains:            vs.GetVirtualHost().GetDomains(),
		virtualHostOptions: vs.GetVirtualHost().GetOptions(),
		sslConfig:          vs.GetSslConfig(),
		routesByName:       make(map[string]*v1.Route, 10),
	}
	for _, r := range vs.GetVirtualHost().GetRoutes() {
		builder.WithRoute(r.GetName(), r)
	}
	return builder
}

func NewVirtualServiceBuilder() *VirtualServiceBuilder {
	return &VirtualServiceBuilder{
		routesByName: make(map[string]*v1.Route, 10),
	}
}

func (b *VirtualServiceBuilder) WithSslConfig(sslConfig *ssl.SslConfig) *VirtualServiceBuilder {
	b.sslConfig = sslConfig
	return b
}

func (b *VirtualServiceBuilder) WithName(name string) *VirtualServiceBuilder {
	b.name = name
	return b
}

func (b *VirtualServiceBuilder) WithNamespace(namespace string) *VirtualServiceBuilder {
	b.namespace = namespace
	return b
}

func (b *VirtualServiceBuilder) WithDomain(domain string) *VirtualServiceBuilder {
	b.domains = []string{domain}
	return b
}

func (b *VirtualServiceBuilder) WithVirtualHostOptions(virtualHostOptions *gloov1.VirtualHostOptions) *VirtualServiceBuilder {
	b.virtualHostOptions = virtualHostOptions
	return b
}

func (b *VirtualServiceBuilder) WithRouteOptions(routeName string, routeOptions *gloov1.RouteOptions) *VirtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Options = routeOptions
	})
}

func (b *VirtualServiceBuilder) WithRoute(routeName string, route *v1.Route) *VirtualServiceBuilder {
	b.routesByName[routeName] = route
	return b
}

func (b *VirtualServiceBuilder) getOrDefaultRoute(routeName string) *v1.Route {
	route, ok := b.routesByName[routeName]
	if !ok {
		return &v1.Route{
			Name: routeName,
		}
	}
	return route
}

func (b *VirtualServiceBuilder) WithRouteActionToUpstream(routeName string, upstream *gloov1.Upstream) *VirtualServiceBuilder {
	return b.WithRouteActionToUpstreamRef(routeName, upstream.GetMetadata().Ref())
}

func (b *VirtualServiceBuilder) WithRouteActionToUpstreamRef(routeName string, upstreamRef *core.ResourceRef) *VirtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Action = &v1.Route_RouteAction{
			RouteAction: &gloov1.RouteAction{
				Destination: &gloov1.RouteAction_Single{
					Single: &gloov1.Destination{
						DestinationType: &gloov1.Destination_Upstream{
							Upstream: upstreamRef,
						},
					},
				},
			},
		}
	})
}

func (b *VirtualServiceBuilder) WithRouteDelegateActionRef(routeName string, delegateRef *core.ResourceRef) *VirtualServiceBuilder {
	return b.WithRouteDelegateAction(routeName,
		&v1.DelegateAction{
			DelegationType: &v1.DelegateAction_Ref{
				Ref: delegateRef,
			},
		})
}

func (b *VirtualServiceBuilder) WithRouteDelegateActionSelector(routeName string, delegateSelector *v1.RouteTableSelector) *VirtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Action = &v1.Route_DelegateAction{
			DelegateAction: &v1.DelegateAction{
				DelegationType: &v1.DelegateAction_Selector{
					Selector: delegateSelector,
				},
			},
		}
	})
}

func (b *VirtualServiceBuilder) WithRouteDelegateAction(routeName string, delegateAction *v1.DelegateAction) *VirtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Action = &v1.Route_DelegateAction{
			DelegateAction: delegateAction,
		}
	})
}

func (b *VirtualServiceBuilder) WithRouteAction(routeName string, routeAction *gloov1.RouteAction) *VirtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Action = &v1.Route_RouteAction{
			RouteAction: routeAction,
		}
	})
}

func (b *VirtualServiceBuilder) WithRouteActionToSingleDestination(routeName string, destination *gloov1.Destination) *VirtualServiceBuilder {
	return b.WithRouteAction(routeName, &gloov1.RouteAction{
		Destination: &gloov1.RouteAction_Single{
			Single: destination,
		},
	})
}

func (b *VirtualServiceBuilder) WithRouteActionToMultiDestination(routeName string, destination *gloov1.MultiDestination) *VirtualServiceBuilder {
	return b.WithRouteAction(routeName, &gloov1.RouteAction{
		Destination: &gloov1.RouteAction_Multi{
			Multi: destination,
		},
	})
}

func (b *VirtualServiceBuilder) WithRouteDirectResponseAction(routeName string, action *gloov1.DirectResponseAction) *VirtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Action = &v1.Route_DirectResponseAction{
			DirectResponseAction: action,
		}
	})
}

func (b *VirtualServiceBuilder) WithRoutePrefixMatcher(routeName string, prefixMatch string) *VirtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Matchers = []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: prefixMatch,
			},
		}}
	})
}

func (b *VirtualServiceBuilder) WithRouteMatcher(routeName string, matcher *matchers.Matcher) *VirtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Matchers = []*matchers.Matcher{matcher}
	})
}

func (b *VirtualServiceBuilder) WithRouteMutation(routeName string, mutation func(route *v1.Route)) *VirtualServiceBuilder {
	route := b.getOrDefaultRoute(routeName)
	mutation(route)
	return b.WithRoute(routeName, route)
}

func (b *VirtualServiceBuilder) errorIfInvalid() error {
	if len(b.domains) == 0 {
		// Unset domains will behave like a wildcard "*", which contributes to test flakes
		return errors.New("attempting to not set a VirtualService domain")

	}
	for _, domain := range b.domains {
		if domain == "*" {
			// Wildcard domains contribute to test flakes
			return errors.New("attempting to set * as a VirtualService domain")
		}
	}
	return nil
}

func (b *VirtualServiceBuilder) Clone() *VirtualServiceBuilder {
	if b == nil {
		return nil
	}
	clone := new(VirtualServiceBuilder)

	clone.name = b.name
	clone.namespace = b.namespace
	clone.domains = nil
	clone.domains = append(clone.domains, b.domains...)
	clone.virtualHostOptions = b.virtualHostOptions.Clone().(*gloov1.VirtualHostOptions)
	clone.routesByName = make(map[string]*v1.Route)
	for key, value := range b.routesByName {
		clone.routesByName[key] = value.Clone().(*v1.Route)
	}
	clone.sslConfig = b.sslConfig.Clone().(*ssl.SslConfig)
	return clone
}

func (b *VirtualServiceBuilder) Build() *v1.VirtualService {
	if err := b.errorIfInvalid(); err != nil {
		// We error loudly here
		// These types of errors are intended to prevent developers from creating resources
		// which are semantically correct, but lead to test flakes/confusion
		ginkgo.Fail(err.Error())
	}

	var routes []*v1.Route
	for _, route := range b.routesByName {
		routes = append(routes, route)
	}

	vs := &v1.VirtualService{
		Metadata: &core.Metadata{
			Name:      b.name,
			Namespace: b.namespace,
		},
		VirtualHost: &v1.VirtualHost{
			Domains: b.domains,
			Routes:  routes,
			Options: b.virtualHostOptions,
		},
		SslConfig: b.sslConfig,
	}
	return proto.Clone(vs).(*v1.VirtualService)
}
