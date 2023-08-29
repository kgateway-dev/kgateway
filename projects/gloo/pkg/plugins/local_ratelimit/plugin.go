package local_ratelimit

import (
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyratelimit "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	envoy_extensions_filters_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_extensions_filters_network_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/local_ratelimit/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	local_ratelimit "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/local_ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"google.golang.org/protobuf/types/known/durationpb"
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
	NetworkFilterStatPrefix = "l4_local_ratelimit"
	HTTPFilterStatPrefix    = "http_local_ratelimit"
	NetworkFilterName       = "envoy.filters.network.local_ratelimit"
	HTTPFilterName          = "envoy.filters.http.local_ratelimit"
	CustomStageBeforeAuth   = uint32(3)
	CustomDomain            = "custom"
)

var (
	// Since this is an L4 filter, it would kick in before any HTTP auth can take place.
	// This also bolsters its main use case which is protect resources.
	pluginStage = plugins.BeforeStage(plugins.AuthNStage)
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

func toEnvoyTokenBucket(localRatelimit *local_ratelimit.TokenBucket) (*envoy_type_v3.TokenBucket, error) {
	if localRatelimit == nil {
		return nil, nil
	}

	tokensPerFill := localRatelimit.GetTokensPerFill()
	if tokensPerFill != nil && tokensPerFill.GetValue() < 1 {
		return nil, fmt.Errorf("TokensPerFill must be greater than or equal to 1. Current value : %v", tokensPerFill.GetValue())
	}

	fillInterval := localRatelimit.GetFillInterval()
	if fillInterval == nil {
		fillInterval = &durationpb.Duration{
			Seconds: 1,
		}
	}

	maxTokens := localRatelimit.GetMaxTokens()
	if maxTokens < 1 {
		return nil, fmt.Errorf("MaxTokens must be greater than or equal to 1. Current value : %v", maxTokens)
	}

	return &envoy_type_v3.TokenBucket{
		MaxTokens:     maxTokens,
		TokensPerFill: tokensPerFill,
		FillInterval:  fillInterval,
	}, nil
}

func generateNetworkFilter(localRatelimit *local_ratelimit.TokenBucket) ([]plugins.StagedNetworkFilter, error) {
	if localRatelimit == nil {
		return []plugins.StagedNetworkFilter{}, nil
	}
	tokenBucket, err := toEnvoyTokenBucket(localRatelimit)
	if err != nil {
		return nil, err
	}

	config := &envoy_extensions_filters_network_local_ratelimit_v3.LocalRateLimit{
		StatPrefix:  NetworkFilterStatPrefix,
		TokenBucket: tokenBucket,
	}
	marshalledConf, err := utils.MessageToAny(config)
	if err != nil {
		return nil, err
	}
	return []plugins.StagedNetworkFilter{
		{
			NetworkFilter: &envoy_config_listener_v3.Filter{
				Name: NetworkFilterName,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: marshalledConf,
				},
			},
			Stage: pluginStage,
		},
	}, nil
}

func (p *plugin) NetworkFiltersHTTP(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedNetworkFilter, error) {
	return generateNetworkFilter(listener.GetOptions().GetNetworkLocalRatelimit())
}

func (p *plugin) NetworkFiltersTCP(params plugins.Params, listener *v1.TcpListener) ([]plugins.StagedNetworkFilter, error) {
	return generateNetworkFilter(listener.GetOptions().GetLocalRatelimit())
}

func generateHTTPFilter(settings *local_ratelimit.Settings, localRatelimit *local_ratelimit.TokenBucket) (*envoy_extensions_filters_http_local_ratelimit_v3.LocalRateLimit, error) {
	tokenBucket, err := toEnvoyTokenBucket(localRatelimit)
	if err != nil {
		return nil, err
	}
	filter := &envoy_extensions_filters_http_local_ratelimit_v3.LocalRateLimit{
		StatPrefix:  HTTPFilterStatPrefix,
		TokenBucket: tokenBucket,
		Stage:       CustomStageBeforeAuth,
	}

	// Do NOT set filter enabled and enforced if the token bucket is not found. This causes it to rate limit all requests to zero
	if tokenBucket != nil {
		filter.FilterEnabled = &corev3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		}
		filter.FilterEnforced = &corev3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		}
	}
	// This needs to be set on every virtual service or route that has custom local RL as they default to false and override the HCM level config
	filter.LocalRateLimitPerDownstreamConnection = settings.GetLocalRateLimitPerDownstreamConnection()
	if settings.GetEnableXRatelimitHeaders() {
		filter.EnableXRatelimitHeaders = envoyratelimit.XRateLimitHeadersRFCVersion_DRAFT_VERSION_03
	}
	return filter, nil
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	if limits := in.GetOptions().GetRatelimit().GetLocalRatelimit(); limits != nil {
		filter, err := generateHTTPFilter(params.HttpListener.GetOptions().GetHttpLocalRatelimit(), limits)
		if err != nil {
			return err
		}
		p.filterRequiredForListener[params.HttpListener] = struct{}{}
		err = pluginutils.SetVhostPerFilterConfig(out, HTTPFilterName, filter)
		return err
	}
	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if limits := in.GetOptions().GetRatelimit().GetLocalRatelimit(); limits != nil {
		filter, err := generateHTTPFilter(params.HttpListener.GetOptions().GetHttpLocalRatelimit(), limits)
		if err != nil {
			return err
		}
		p.filterRequiredForListener[params.HttpListener] = struct{}{}
		err = pluginutils.SetRoutePerFilterConfig(out, HTTPFilterName, filter)
		return err
	}
	return nil
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	settings := listener.GetOptions().GetHttpLocalRatelimit()
	filter, err := generateHTTPFilter(settings, settings.GetDefaults())
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
		pluginStage,
	)
	if err != nil {
		return nil, err
	}

	return []plugins.StagedHttpFilter{
		stagedRateLimitFilter,
	}, nil
}
