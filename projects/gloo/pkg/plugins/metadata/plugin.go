package metadata

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// Sets [static metadata](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/advanced/data_sharing_between_filters#metadata)
// on Envoy v3.Route objects.
type Plugin struct{}

var _ plugins.RoutePlugin = NewPlugin()

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(_ plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessRoute(_ plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {

	if envoyMetadata := in.GetOptions().GetEnvoyMetadata(); len(envoyMetadata) > 0 {
		out.Metadata = &envoy_config_core_v3.Metadata{
			FilterMetadata: envoyMetadata,
		}
	}

	return nil
}
