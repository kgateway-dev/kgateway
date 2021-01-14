package wasm

//go:generate mockgen -destination mocks/mock_cache.go  github.com/solo-io/wasm/tools/wasme/pkg/cache Cache

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	"github.com/rotisserie/eris"
)

// Compile-time assertion
var _ plugins.Plugin = &Plugin{}
var _ plugins.HttpFilterPlugin = &Plugin{}
var _ plugins.Upgradable = &Plugin{}

const errEnterpriseOnly = "Could not load wasm plugin - this is an Enterprise feature"
const pluginName = "wasm"

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) PluginName() string {
	return pluginName
}

func (p *Plugin) IsUpgrade() bool {
	return false
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) HttpFilters(params plugins.Params, l *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	wasm := l.GetOptions().GetWasm()
	if wasm != nil {
		return nil, eris.New(errEnterpriseOnly)
	}
	return nil, nil
}
