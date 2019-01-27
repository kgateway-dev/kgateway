package transformation

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	FilterName  = "io.solo.transformation"
	pluginStage = plugins.PostInAuth
)

type Plugin struct {
	RequireTransformationFilter bool
}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	p.RequireTransformationFilter = false
	return nil
}

// TODO(yuval-k): We need to figure out what\if to do in edge cases where there is cluster weight transform
func (p *Plugin) ProcessRoute(params plugins.Params, in *v1.Route, out *envoyroute.Route) error {
	if in.RoutePlugins == nil {
		return nil
	}
	if in.RoutePlugins.Transformations == nil {
		return nil
	}
	if out.PerFilterConfig == nil {
		out.PerFilterConfig = make(map[string]*types.Struct)
	}

	configStruct, err := util.MessageToStruct(in.RoutePlugins.Transformations)
	if err != nil {
		return err
	}

	p.RequireTransformationFilter = true
	out.PerFilterConfig[FilterName] = configStruct
	return nil
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	return []plugins.StagedHttpFilter{
		{
			HttpFilter: &envoyhttp.HttpFilter{Name: FilterName},
			Stage:      pluginStage,
		},
	}, nil
}
