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
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	local_ratelimit "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/local_ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"google.golang.org/protobuf/types/known/durationpb"
)

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
	} else {
		// It should be a valid time >= 50ms
		if fillInterval.GetSeconds() == 0 && fillInterval.GetNanos() < 50000000 {
			return nil, fmt.Errorf("FillInterval must be >= 50ms. Current value : %vms", fillInterval.GetNanos()/1000000)
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

// This function exported since it is used in the enterprise plugin
func GenerateHTTPFilter(settings *local_ratelimit.Settings, localRatelimit *local_ratelimit.TokenBucket, stage uint32) (*envoy_extensions_filters_http_local_ratelimit_v3.LocalRateLimit, error) {
	tokenBucket, err := toEnvoyTokenBucket(localRatelimit)
	if err != nil {
		return nil, err
	}
	filter := &envoy_extensions_filters_http_local_ratelimit_v3.LocalRateLimit{
		StatPrefix:  HTTPFilterStatPrefix,
		TokenBucket: tokenBucket,
		Stage:       stage,
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
	filter.LocalRateLimitPerDownstreamConnection = settings.GetLocalRateLimitPerDownstreamConnection().GetValue()
	if settings.GetEnableXRatelimitHeaders().GetValue() {
		filter.EnableXRatelimitHeaders = envoyratelimit.XRateLimitHeadersRFCVersion_DRAFT_VERSION_03
	}
	return filter, nil
}

func modIfNoExisting(protoext proto.Message) pluginutils.ModifyFunc {
	return func(existing *any.Any) (proto.Message, error) {
		if existing == nil {
			return protoext, nil
		}
		return nil, ErrConfigurationExists
	}
}

// This function exported since it is used in the enterprise plugin
func ConfigureVirtualHostFilter(settings *local_ratelimit.Settings, localRatelimit *local_ratelimit.TokenBucket, stage uint32, out *envoy_config_route_v3.VirtualHost) error {
	filter, err := GenerateHTTPFilter(settings, localRatelimit, stage)
	if err != nil {
		return err
	}
	// Mark this stage as having user-defined configuration
	return pluginutils.ModifyVhostPerFilterConfig(out, HTTPFilterName, modIfNoExisting(filter))
}

// This function exported since it is used in the enterprise plugin
func ConfigureRouteFilter(settings *local_ratelimit.Settings, localRatelimit *local_ratelimit.TokenBucket, stage uint32, out *envoy_config_route_v3.Route) error {
	filter, err := GenerateHTTPFilter(settings, localRatelimit, stage)
	if err != nil {
		return err
	}
	// Mark this stage as having user-defined configuration
	return pluginutils.ModifyRoutePerFilterConfig(out, HTTPFilterName, modIfNoExisting(filter))
}
