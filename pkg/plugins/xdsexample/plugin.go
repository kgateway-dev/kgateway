package xdsexample

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins"
)


//TODO: delete me
func init() {
	plugins.Register(&Plugin{})
}

type Plugin struct{}

func (p *Plugin) GetDependencies(_ *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *Plugin) CallbackFunc() {
}
