package helpers

import (
	"github.com/golang/protobuf/proto"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	ExactPath = iota
	PrefixPath
	RegexPath
)

func MakeMultiMatcherRoute(pathType1, length1, pathType2, length2 int) *gloov1.Route {
	return &gloov1.Route{Matchers: []*matchers.Matcher{
		MakeMatcher(pathType1, length1),
		MakeMatcher(pathType2, length2),
	}}
}

func MakeRoute(pathType, length int) *gloov1.Route {
	return &gloov1.Route{Matchers: []*matchers.Matcher{MakeMatcher(pathType, length)}}
}

func MakeGatewayRoute(pathType, length int) *gatewayv1.Route {
	return &gatewayv1.Route{Matchers: []*matchers.Matcher{MakeMatcher(pathType, length)}}
}

func MakeMatcher(pathType, length int) *matchers.Matcher {
	pathStr := "/"
	for i := 0; i < length; i++ {
		pathStr += "s/"
	}
	m := &matchers.Matcher{}
	switch pathType {
	case ExactPath:
		m.PathSpecifier = &matchers.Matcher_Exact{pathStr}
	case PrefixPath:
		m.PathSpecifier = &matchers.Matcher_Prefix{pathStr}
	case RegexPath:
		m.PathSpecifier = &matchers.Matcher_Regex{pathStr}
	default:
		panic("bad test")
	}
	return m
}

// RouteBuilder simplifies the process of generating Routes in tests
type RouteBuilder struct {
	name         string
	matchers     []*matchers.Matcher
	routeOptions *gloov1.RouteOptions
	routeAction  *gloov1.RouteAction
}

// BuilderFromRoute creates a new RouteBuilder from an existing Route
func BuilderFromRoute(r *gatewayv1.Route) *RouteBuilder {
	builder := &RouteBuilder{
		name:         r.GetName(),
		routeOptions: r.GetOptions(),
		routeAction:  r.GetRouteAction(),
		matchers:     make([]*matchers.Matcher, len(r.GetMatchers())),
	}
	for _, m := range r.GetMatchers() {
		builder.WithMatcher(m)
	}
	return builder
}

// NewRouteBuilder creates an empty RouteBuilder
func NewRouteBuilder() *RouteBuilder {
	return &RouteBuilder{
		matchers: make([]*matchers.Matcher, 0),
	}
}

func (b *RouteBuilder) WithName(name string) *RouteBuilder {
	b.name = name
	return b
}

func (b *RouteBuilder) WithMatcher(matcher *matchers.Matcher) *RouteBuilder {
	b.matchers = append(b.matchers, matcher)
	return b
}

func (b *RouteBuilder) WithPrefixMatcher(prefix string) *RouteBuilder {
	prefixMatch := &matchers.Matcher{
		PathSpecifier: &matchers.Matcher_Prefix{Prefix: prefix},
	}
	b.matchers = append(b.matchers, prefixMatch)
	return b
}

func (b *RouteBuilder) WithRouteOptions(opts *gloov1.RouteOptions) *RouteBuilder {
	b.routeOptions = opts
	return b
}

func (b *RouteBuilder) WithRouteAction(routeAction *gloov1.RouteAction) *RouteBuilder {
	b.routeAction = routeAction
	return b
}

func (b *RouteBuilder) WithRouteActionToUpstreamRef(ref *core.ResourceRef) *RouteBuilder {
	b.routeAction = &gloov1.RouteAction{
		Destination: &gloov1.RouteAction_Single{
			Single: &gloov1.Destination{
				DestinationType: &gloov1.Destination_Upstream{
					Upstream: ref,
				},
			},
		},
	}
	return b
}

func (b *RouteBuilder) Clone() *RouteBuilder {
	if b == nil {
		return nil
	}
	clone := new(RouteBuilder)

	clone.name = b.name
	clone.routeOptions = b.routeOptions
	clone.routeAction = b.routeAction

	clone.matchers = make([]*matchers.Matcher, len(b.matchers))
	for i, m := range b.matchers {
		clone.matchers[i] = m.Clone().(*matchers.Matcher)
	}

	return clone
}

func (b *RouteBuilder) Build() *gatewayv1.Route {
	action := &gatewayv1.Route_RouteAction{
		RouteAction: b.routeAction,
	}
	rt := &gatewayv1.Route{
		Name:     b.name,
		Matchers: b.matchers,
		Options:  b.routeOptions,
		Action:   action,
	}
	return proto.Clone(rt).(*gatewayv1.Route)
}
