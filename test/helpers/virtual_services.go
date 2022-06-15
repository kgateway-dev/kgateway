package helpers

import (
	"github.com/golang/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// virtualServiceBuilder simplifies the process of generating VirtualServices in tests
type virtualServiceBuilder struct {
	name      string
	namespace string

	domains      []string
	routesByName map[string]*v1.Route
	sslConfig    *gloov1.SslConfig
}

func NewVirtualServiceBuilder() *virtualServiceBuilder {
	return &virtualServiceBuilder{
		routesByName: make(map[string]*v1.Route, 10),
	}
}

func (b *virtualServiceBuilder) WithSslConfig(sslConfig *gloov1.SslConfig) *virtualServiceBuilder {
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

func (b *virtualServiceBuilder) WithDomains(domains []string) *virtualServiceBuilder {
	b.domains = domains
	return b
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
	route := b.getOrDefaultRoute(routeName)

	route.Action = &v1.Route_RouteAction{
		RouteAction: &gloov1.RouteAction{
			Destination: &gloov1.RouteAction_Single{
				Single: &gloov1.Destination{
					DestinationType: &gloov1.Destination_Upstream{
						Upstream: upstream.GetMetadata().Ref(),
					},
				},
			},
		},
	}
	return b.WithRoute(routeName, route)
}

func (b *virtualServiceBuilder) WithPrefixMatcher(routeName string, prefixMatch string) *virtualServiceBuilder {
	route := b.getOrDefaultRoute(routeName)

	route.Matchers = []*matchers.Matcher{{
		PathSpecifier: &matchers.Matcher_Prefix{
			Prefix: prefixMatch,
		},
	}}
	return b.WithRoute(routeName, route)
}

func (b *virtualServiceBuilder) Build() *v1.VirtualService {
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
			Options: nil,
		},
		SslConfig: b.sslConfig,
	}
	return proto.Clone(vs).(*v1.VirtualService)
}
