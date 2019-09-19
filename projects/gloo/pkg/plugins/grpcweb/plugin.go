package grpcweb

import (
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// filter info
var pluginStage = plugins.AfterStage(plugins.AuthZStage)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.HttpFilterPlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func isDisabled(httplistener *v1.HttpListener) bool {
	if httplistener == nil {
		return false
	}
	listenerplugins := httplistener.GetListenerPlugins()
	if listenerplugins == nil {
		return false
	}
	grpcweb := listenerplugins.GetGrpcWeb()
	if grpcweb == nil {
		return false
	}
	return grpcweb.Disable
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if isDisabled(listener) {
		return nil, nil
	}
	return []plugins.StagedHttpFilter{
		plugins.NewStagedFilter(envoyutil.GRPCWeb, pluginStage),
	}, nil
}
