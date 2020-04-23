package grpchttp1reversebridge

import (
	envoyapiv2route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	FilterName = "envoy.filters.http.grpc_http1_reverse_bridge"
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

type Plugin struct {
}

func (p Plugin) Init(params plugins.InitParams) error {
	panic("implement me")
}

func (p Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	panic("implement me")
}

func (p Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyapiv2route.Route) error {
	panic("implement me")
}
