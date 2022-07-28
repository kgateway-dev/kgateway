package buffer

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoybuffer "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/rotisserie/eris"
	buffer "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var (
	_ plugins.Plugin                    = new(plugin)
	_ plugins.HttpFilterPlugin          = new(plugin)
	_ plugins.RoutePlugin               = new(plugin)
	_ plugins.VirtualHostPlugin         = new(plugin)
	_ plugins.WeightedDestinationPlugin = new(plugin)
)

const (
	ExtensionName = "buffer"
)

// filter should be called after routing decision has been made
var pluginStage = plugins.DuringStage(plugins.RouteStage)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) HttpFilters(_ plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	bufferConfig := p.translateBufferFilter(listener.GetOptions().GetBuffer())

	if bufferConfig == nil {
		return nil, nil
	}

	bufferFilter, err := plugins.NewStagedFilter(wellknown.Buffer, bufferConfig, pluginStage)
	if err != nil {
		return nil, eris.Wrapf(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{bufferFilter}, nil
}

func (p *plugin) translateBufferFilter(buf *buffer.Buffer) *envoybuffer.Buffer {
	if buf == nil {
		return nil
	}

	return &envoybuffer.Buffer{
		MaxRequestBytes: buf.GetMaxRequestBytes(),
	}
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	bufPerRoute := in.GetOptions().GetBufferPerRoute()
	if bufPerRoute == nil {
		return nil
	}

	if bufPerRoute.GetDisabled() {
		return pluginutils.SetRoutePerFilterConfig(out, wellknown.Buffer, getNoBufferConfig())
	}

	if bufPerRoute.GetBuffer() != nil {
		config, err := getBufferConfig(bufPerRoute)
		if err != nil {
			return err
		}
		return pluginutils.SetRoutePerFilterConfig(out, wellknown.Buffer, config)
	}

	return nil
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	bufPerRoute := in.GetOptions().GetBufferPerRoute()
	if bufPerRoute == nil {
		return nil
	}

	if bufPerRoute.GetDisabled() {
		return pluginutils.SetVhostPerFilterConfig(out, wellknown.Buffer, getNoBufferConfig())
	}

	if bufPerRoute.GetBuffer() != nil {
		config, err := getBufferConfig(bufPerRoute)
		if err != nil {
			return err
		}
		return pluginutils.SetVhostPerFilterConfig(out, wellknown.Buffer, config)
	}

	return nil
}

func (p *plugin) ProcessWeightedDestination(
	params plugins.RouteParams,
	in *v1.WeightedDestination,
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
) error {
	bufPerRoute := in.GetOptions().GetBufferPerRoute()
	if bufPerRoute == nil {
		return nil
	}

	if bufPerRoute.GetDisabled() {
		return pluginutils.SetWeightedClusterPerFilterConfig(out, wellknown.Buffer, getNoBufferConfig())
	}

	if bufPerRoute.GetBuffer() != nil {
		config, err := getBufferConfig(bufPerRoute)
		if err != nil {
			return err
		}
		return pluginutils.SetWeightedClusterPerFilterConfig(out, wellknown.Buffer, config)
	}

	return nil
}

func getNoBufferConfig() *envoybuffer.BufferPerRoute {
	return &envoybuffer.BufferPerRoute{
		Override: &envoybuffer.BufferPerRoute_Disabled{
			Disabled: true,
		},
	}
}

func getBufferConfig(bufPerRoute *buffer.BufferPerRoute) (*envoybuffer.BufferPerRoute, error) {
	envoyConfig := &envoybuffer.BufferPerRoute{
		Override: &envoybuffer.BufferPerRoute_Buffer{
			Buffer: &envoybuffer.Buffer{
				MaxRequestBytes: bufPerRoute.GetBuffer().GetMaxRequestBytes(),
			},
		},
	}
	return envoyConfig, envoyConfig.Validate()
}
