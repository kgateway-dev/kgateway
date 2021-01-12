package dlp

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	"github.com/rotisserie/eris"
)

// Compile-time assertion
var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
	_ plugins.Upgradable       = new(plugin)
)

const (
	ErrEnterpriseOnly = "Could not load dlp plugin - this is an Enterprise feature"
	ExtensionName     = "dlp"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) PluginName() string {
	return ExtensionName
}

func (p *plugin) IsUpgrade() bool {
	return false
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) HttpFilters(params plugins.Params, l *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if l.GetOptions().GetDlp() != nil {
		return nil, eris.New(ErrEnterpriseOnly)
	}
	return nil, nil
}
