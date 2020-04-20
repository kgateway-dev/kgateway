package listener

import (
	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.ListenerPlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(_ plugins.InitParams) error {
	return nil
}

// Used to set config that are directly on the [Envoy listener](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/listener.proto)
func (p *Plugin) ProcessListener(_ plugins.Params, in *v1.Listener, out *envoy_api_v2.Listener) error {
	if in.GetOptions().GetPerConnectionBufferLimitBytes() == nil || in.GetOptions().GetPerConnectionBufferLimitBytes().Value == 0 {
		// Rely on default behavior
		return nil
	}

	out.PerConnectionBufferLimitBytes = gogoutils.UInt32GogoToProto(in.GetOptions().PerConnectionBufferLimitBytes)
	return nil
}
