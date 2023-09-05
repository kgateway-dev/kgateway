package local_ratelimit

import (
	"errors"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var (
	_ plugins.Plugin              = new(plugin)
	_ plugins.NetworkFilterPlugin = new(plugin)
	_ plugins.HttpFilterPlugin    = new(plugin)
	_ plugins.VirtualHostPlugin   = new(plugin)
	_ plugins.RoutePlugin         = new(plugin)
)

const (
	ExtensionName           = "local_ratelimit"
	NetworkFilterStatPrefix = "network_local_ratelimit"
	HTTPFilterStatPrefix    = "http_local_ratelimit"
	NetworkFilterName       = "envoy.filters.network.local_ratelimit"
	HTTPFilterName          = "envoy.filters.http.local_ratelimit"
	CustomStageBeforeAuth   = uint32(3)
)

var (
	// For the network filter, it would kick in after the TCP connection limit filter to rate limit.
	networkFilterPluginStage = plugins.DuringStage(plugins.RateLimitStage)
	// For the HTTP filter, this is designed to rate limit early, and consequently kick in before auth
	httpFilterPluginStage = plugins.BeforeStage(plugins.AuthNStage)

	ErrConfigurationExists = errors.New("configuration already exists")
)

type plugin struct {
	removeUnused              bool
	filterRequiredForListener map[*v1.HttpListener]struct{}
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.removeUnused = params.Settings.GetGloo().GetRemoveUnusedFilters().GetValue()
	p.filterRequiredForListener = make(map[*v1.HttpListener]struct{})
}

func (p *plugin) NetworkFiltersHTTP(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedNetworkFilter, error) {
	return generateNetworkFilter(listener.GetOptions().GetNetworkLocalRatelimit())
}

func (p *plugin) NetworkFiltersTCP(params plugins.Params, listener *v1.TcpListener) ([]plugins.StagedNetworkFilter, error) {
	return generateNetworkFilter(listener.GetOptions().GetLocalRatelimit())
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	if limits := in.GetOptions().GetRatelimit().GetLocalRatelimit(); limits != nil {
		err := ConfigureVirtualHostFilter(params.HttpListener.GetOptions().GetHttpLocalRatelimit(), limits, CustomStageBeforeAuth, out)
		if err != nil {
			return err
		}
		p.filterRequiredForListener[params.HttpListener] = struct{}{}
		return nil
	}
	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if limits := in.GetOptions().GetRatelimit().GetLocalRatelimit(); limits != nil {
		err := ConfigureRouteFilter(params.HttpListener.GetOptions().GetHttpLocalRatelimit(), limits, CustomStageBeforeAuth, out)
		if err != nil {
			return err
		}
		p.filterRequiredForListener[params.HttpListener] = struct{}{}
	}
	return nil
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	settings := listener.GetOptions().GetHttpLocalRatelimit()
	filter, err := GenerateHTTPFilter(settings, settings.GetDefaultLimit(), CustomStageBeforeAuth)
	if err != nil {
		return nil, err
	}

	// Do NOT add this filter if all of the following are met :
	// - It is not used on this listener either at the vhost, route level &&
	// - The token bucket is not defined at the gateway level &&
	// - params.Settings.GetGloo().GetRemoveUnusedFilters() is set
	_, ok := p.filterRequiredForListener[listener]
	if !ok && p.removeUnused && filter.GetTokenBucket() == nil {
		return []plugins.StagedHttpFilter{}, nil
	}

	stagedRateLimitFilter, err := plugins.NewStagedFilter(
		HTTPFilterName,
		filter,
		httpFilterPluginStage,
	)
	if err != nil {
		return nil, err
	}

	return []plugins.StagedHttpFilter{
		stagedRateLimitFilter,
	}, nil
}
