package grpcweb

import (
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
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
	disabled bool
}

func (p *Plugin) Init(params plugins.InitParams) error {
	maybeDisabled := params.Settings.GetGloo().GetDisableGrpcWeb()
	if maybeDisabled != nil {
		p.disabled = maybeDisabled.GetValue()
	} else {
		// default to true if not specified
		p.disabled = false
	}
	return nil
}

func (p *Plugin) isDisabled(httplistener *v1.HttpListener) bool {
	if httplistener == nil {
		return p.disabled
	}
	listenerplugins := httplistener.GetOptions()
	if listenerplugins == nil {
		return p.disabled
	}
	grpcweb := listenerplugins.GetGrpcWeb()
	if grpcweb == nil {
		return p.disabled
	}
	return grpcweb.GetDisable()
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if p.isDisabled(listener) {
		return nil, nil
	}
	return []plugins.StagedHttpFilter{
		plugins.NewStagedFilter(wellknown.GRPCWeb, pluginStage),
	}, nil
}
