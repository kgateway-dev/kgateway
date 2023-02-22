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

// virtualServiceBuilder simplifies the process of generating VirtualServices in tests
type virtualServiceBuilder struct {
	name      string
	namespace string

	domains            []string
	virtualHostOptions *gloov1.VirtualHostOptions
	routesByName       map[string]*v1.Route
	sslConfig          *ssl.SslConfig
}

func BuilderFromVirtualService(vs *v1.VirtualService) *virtualServiceBuilder {
	builder := &virtualServiceBuilder{
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

func NewVirtualServiceBuilder() *virtualServiceBuilder {
	return &virtualServiceBuilder{
		routesByName: make(map[string]*v1.Route, 10),
	}
}

func (b *virtualServiceBuilder) WithSslConfig(sslConfig *ssl.SslConfig) *virtualServiceBuilder {
	b.sslConfig = sslConfig
	return b
}

func (b *virtualServiceBuilder) WithName(name string) *virtualServiceBuilder {
	b.name = name
	return b
}

func (b *virtualServiceBuilder) WithNamespace(namespace string) *virtualServiceBuilder {
	b.namespace = namespace
	return b
}

func (b *virtualServiceBuilder) WithDomain(domain string) *virtualServiceBuilder {
	b.domains = []string{domain}
	return b
}

func (b *virtualServiceBuilder) WithVirtualHostOptions(virtualHostOptions *gloov1.VirtualHostOptions) *virtualServiceBuilder {
	b.virtualHostOptions = virtualHostOptions
	return b
}

func (b *virtualServiceBuilder) WithRouteOptions(routeName string, routeOptions *gloov1.RouteOptions) *virtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Options = routeOptions
	})
}

func (b *virtualServiceBuilder) WithRoute(routeName string, route *v1.Route) *virtualServiceBuilder {
	b.routesByName[routeName] = route
	return b
}

func (b *virtualServiceBuilder) getOrDefaultRoute(routeName string) *v1.Route {
	route, ok := b.routesByName[routeName]
	if !ok {
		return &v1.Route{
			Name: routeName,
		}
	}
	return route
}

func (b *virtualServiceBuilder) WithRouteActionToUpstream(routeName string, upstream *gloov1.Upstream) *virtualServiceBuilder {
	return b.WithRouteActionToUpstreamRef(routeName, upstream.GetMetadata().Ref())
}

func (b *virtualServiceBuilder) WithRouteActionToUpstreamRef(routeName string, upstreamRef *core.ResourceRef) *virtualServiceBuilder {
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

func (b *virtualServiceBuilder) WithRouteDelegateActionRef(routeName string, delegateRef *core.ResourceRef) *virtualServiceBuilder {
	return b.WithRouteDelegateAction(routeName,
		&v1.DelegateAction{
			DelegationType: &v1.DelegateAction_Ref{
				Ref: delegateRef,
			},
		})
}

func (b *virtualServiceBuilder) WithRouteDelegateActionSelector(routeName string, delegateSelector *v1.RouteTableSelector) *virtualServiceBuilder {
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

func (b *virtualServiceBuilder) WithRouteDelegateAction(routeName string, delegateAction *v1.DelegateAction) *virtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Action = &v1.Route_DelegateAction{
			DelegateAction: delegateAction,
		}
	})
}

func (b *virtualServiceBuilder) WithRouteAction(routeName string, routeAction *gloov1.RouteAction) *virtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Action = &v1.Route_RouteAction{
			RouteAction: routeAction,
		}
	})
}

func (b *virtualServiceBuilder) WithRouteActionToSingleDestination(routeName string, destination *gloov1.Destination) *virtualServiceBuilder {
	return b.WithRouteAction(routeName, &gloov1.RouteAction{
		Destination: &gloov1.RouteAction_Single{
			Single: destination,
		},
	})
}

func (b *virtualServiceBuilder) WithRouteActionToMultiDestination(routeName string, destination *gloov1.MultiDestination) *virtualServiceBuilder {
	return b.WithRouteAction(routeName, &gloov1.RouteAction{
		Destination: &gloov1.RouteAction_Multi{
			Multi: destination,
		},
	})
}

func (b *virtualServiceBuilder) WithRouteDirectResponseAction(routeName string, action *gloov1.DirectResponseAction) *virtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Action = &v1.Route_DirectResponseAction{
			DirectResponseAction: action,
		}
	})
}

func (b *virtualServiceBuilder) WithRoutePrefixMatcher(routeName string, prefixMatch string) *virtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Matchers = []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: prefixMatch,
			},
		}}
	})
}

func (b *virtualServiceBuilder) WithRouteMatcher(routeName string, matcher *matchers.Matcher) *virtualServiceBuilder {
	return b.WithRouteMutation(routeName, func(route *v1.Route) {
		route.Matchers = []*matchers.Matcher{matcher}
	})
}

func (b *virtualServiceBuilder) WithRouteMutation(routeName string, mutation func(route *v1.Route)) *virtualServiceBuilder {
	route := b.getOrDefaultRoute(routeName)
	mutation(route)
	return b.WithRoute(routeName, route)
}

func (b *virtualServiceBuilder) errorIfInvalid() error {
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

func (b *virtualServiceBuilder) Build() *v1.VirtualService {
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
