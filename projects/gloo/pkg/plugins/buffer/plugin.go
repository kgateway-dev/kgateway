package buffer

import (
	"github.com/rotisserie/eris"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoybuffer "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"

	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	buffer "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

// filter should be called after routing decision has been made
var pluginStage = plugins.DuringStage(plugins.RouteStage)

const FilterName = "envoy.filters.http.buffer"

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

	bufferFilter, err := plugins.NewStagedFilterWithConfig(FilterName, bufferConfig, pluginStage)
	if err != nil {
		return nil, eris.Wrapf(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{bufferFilter}, nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	bufPerRoute := in.Options.GetBufferPerRoute()
	if bufPerRoute == nil {
		return nil
	}

	if bufPerRoute.GetDisabled() {
		return pluginutils.SetRoutePerFilterConfig(out, FilterName, getNoBufferConfig())
	}

	if bufPerRoute.GetBuffer() != nil {
		config := getBufferConfig(bufPerRoute)
		return pluginutils.SetRoutePerFilterConfig(out, FilterName, config)
	}

	return nil
}

func (p *Plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	bufPerRoute := in.GetOptions().GetBufferPerRoute()
	if bufPerRoute == nil {
		return nil
	}

	if bufPerRoute.GetDisabled() {
		return pluginutils.SetVhostPerFilterConfig(out, FilterName, getNoBufferConfig())
	}

	if bufPerRoute.GetBuffer() != nil {
		config := getBufferConfig(bufPerRoute)
		return pluginutils.SetVhostPerFilterConfig(out, FilterName, config)
	}

	return nil
}

func (p *Plugin) ProcessWeightedDestination(params plugins.RouteParams, in *v1.WeightedDestination, out *envoyroute.WeightedCluster_ClusterWeight) error {
	bufPerRoute := in.GetOptions().GetBufferPerRoute()
	if bufPerRoute == nil {
		return nil
	}

	if bufPerRoute.GetDisabled() {
		return pluginutils.SetWeightedClusterPerFilterConfig(out, FilterName, getNoBufferConfig())
	}

	if bufPerRoute.GetBuffer() != nil {
		config := getBufferConfig(bufPerRoute)
		return pluginutils.SetWeightedClusterPerFilterConfig(out, FilterName, config)
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

func getBufferConfig(bufPerRoute *buffer.BufferPerRoute) *envoybuffer.BufferPerRoute {
	return &envoybuffer.BufferPerRoute{
		Override: &envoybuffer.BufferPerRoute_Buffer{
			Buffer: &envoybuffer.Buffer{
				MaxRequestBytes: gogoutils.UInt32GogoToProto(bufPerRoute.GetBuffer().GetMaxRequestBytes()),
			},
		},
	}
}
