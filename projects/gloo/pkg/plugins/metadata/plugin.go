package metadata

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var (
	_ plugins.RoutePlugin = new(plugin)
)

const (
	ExtensionName = "metadata"
)

// Sets [static metadata](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/advanced/data_sharing_between_filters#metadata)
// on Envoy v3.Route objects.
type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) ProcessRoute(_ plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if envoyMetadata := in.GetOptions().GetEnvoyMetadata(); len(envoyMetadata) > 0 {
		out.Metadata = &envoy_config_core_v3.Metadata{
			FilterMetadata: envoyMetadata,
		}
	}

	return nil
}
