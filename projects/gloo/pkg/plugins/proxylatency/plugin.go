package proxylatency

import (
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	FilterName = "io.solo.filters.http.proxy_latency"
    errEnterpriseOnly = "Could not load dlp plugin - this is an Enterprise feature"
	pluginName = "dlp"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)

	// This filter must be last as it is used to measure latency of all the other filters.
	FilterStage = plugins.AfterStage(plugins.RouteStage)
)

type plugin struct {
}

var _ plugins.Plugin = new(plugin)

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) PluginName() string {
	return pluginName
}

func (p *plugin) IsUpgrade() bool {
	return false
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if pl := listener.GetOptions().GetProxyLatency();
	pl != nil {
		return nil, eris.New(errEnterpriseOnly)
	}

	return nil, nil
}
