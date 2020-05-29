package buffer

import (
	"github.com/rotisserie/eris"

	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/util"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// filter should be called after routing decision has been made
var pluginStage = plugins.DuringStage(plugins.RouteStage)

const filterName = util.Buffer

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

func (p *Plugin) HttpFilters(_ plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {

	bufferConfig := listener.GetOptions().GetBuffer()

	if bufferConfig == nil {
		return nil, nil
	}

	bufferFilter, err := plugins.NewStagedFilterWithConfig(filterName, bufferConfig, pluginStage)
	if err != nil {
		return nil, eris.Wrapf(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{bufferFilter}, nil
}
