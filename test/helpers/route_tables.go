package helpers

import (
	"github.com/golang/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// RouteTableBuilder simplifies the process of generating RouteTables in tests
type RouteTableBuilder struct {
	name         string
	namespace    string
	routesByName map[string]*v1.Route
}

// BuilderFromRouteTable creates a new RouteTableBuilder from an existing RouteTable
func BuilderFromRouteTable(rt *v1.RouteTable) *RouteTableBuilder {
	builder := &RouteTableBuilder{
		name:         rt.GetMetadata().GetName(),
		namespace:    rt.GetMetadata().GetNamespace(),
		routesByName: make(map[string]*v1.Route, len(rt.GetRoutes())),
	}
	for _, r := range rt.GetRoutes() {
		builder.WithRoute(r.GetName(), r)
	}
	return builder
}

// NewRouteTableBuilder creates an empty RouteTableBuilder
func NewRouteTableBuilder() *RouteTableBuilder {
	return &RouteTableBuilder{
		routesByName: make(map[string]*v1.Route, 10),
	}
}

func (b *RouteTableBuilder) WithName(name string) *RouteTableBuilder {
	b.name = name
	return b
}

func (b *RouteTableBuilder) WithNamespace(namespace string) *RouteTableBuilder {
	b.namespace = namespace
	return b
}

func (b *RouteTableBuilder) WithRoute(name string, route *v1.Route) *RouteTableBuilder {
	b.routesByName[name] = route
	return b
}

func (b *RouteTableBuilder) Clone() *RouteTableBuilder {
	if b == nil {
		return nil
	}
	clone := new(RouteTableBuilder)

	clone.name = b.name
	clone.namespace = b.namespace

	clone.routesByName = make(map[string]*v1.Route, len(b.routesByName))
	for key, value := range b.routesByName {
		clone.routesByName[key] = value.Clone().(*v1.Route)
	}
	return clone
}

func (b *RouteTableBuilder) Build() *v1.RouteTable {
	var routes []*v1.Route
	for _, route := range b.routesByName {
		routes = append(routes, route)
	}

	rt := &v1.RouteTable{
		Metadata: &core.Metadata{
			Name:      b.name,
			Namespace: b.namespace,
		},
		Routes: routes,
	}
	return proto.Clone(rt).(*v1.RouteTable)
}
