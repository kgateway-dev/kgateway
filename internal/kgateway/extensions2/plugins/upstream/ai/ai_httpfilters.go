package ai

import (
	"time"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	upstream_wait "github.com/solo-io/envoy-gloo/go/config/filter/http/upstream_wait/v2"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func AddUpstreamHttpFilters() ([]plugins.StagedUpstreamHttpFilter, error) {
	transformationMsg, err := utils.MessageToAny(&envoytransformation.FilterTransformations{})
	if err != nil {
		return nil, err
	}

	upstreamWaitMsg, err := utils.MessageToAny(&upstream_wait.UpstreamWaitFilterConfig{})
	if err != nil {
		return nil, err
	}

	filters := []plugins.StagedUpstreamHttpFilter{
		// The wait filter essentially blocks filter iteration until a host has been selected.
		// This is important because running as an upstream filter allows access to host
		// metadata iff the host has already been selected, and that's a
		// major benefit of running the filter at this stage.
		{
			Filter: &envoy_hcm.HttpFilter{
				Name: waitFilterName,
				ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
					TypedConfig: upstreamWaitMsg,
				},
			},
			Stage: plugins.UpstreamHTTPFilterStage{
				RelativeTo: plugins.TransformationStage,
				Weight:     -1,
			},
		},
		{
			Filter: &envoy_hcm.HttpFilter{
				Name: wellknown.TransformationFilterName,
				ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
					TypedConfig: transformationMsg,
				},
			},
			Stage: plugins.UpstreamHTTPFilterStage{
				RelativeTo: plugins.TransformationStage,
				Weight:     0,
			},
		},
	}
	return filters, nil
}

func AddExtprocHTTPFilter() ([]plugins.StagedHttpFilter, error) {
	result := []plugins.StagedHttpFilter{}

	// TODO: add ratelimit and jwt_authn if AI Upstream is configured
	extProcSettings := &envoy_ext_proc_v3.ExternalProcessor{
		GrpcService: &envoy_config_core_v3.GrpcService{
			Timeout: durationpb.New(5 * time.Second),
			RetryPolicy: &envoy_config_core_v3.RetryPolicy{
				NumRetries: wrapperspb.UInt32(3),
			},
			TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
					ClusterName: extProcUDSClusterName,
				},
			},
		},
		ProcessingMode: &envoy_ext_proc_v3.ProcessingMode{
			RequestHeaderMode:   envoy_ext_proc_v3.ProcessingMode_SEND,
			RequestBodyMode:     envoy_ext_proc_v3.ProcessingMode_STREAMED,
			RequestTrailerMode:  envoy_ext_proc_v3.ProcessingMode_SKIP,
			ResponseHeaderMode:  envoy_ext_proc_v3.ProcessingMode_SEND,
			ResponseBodyMode:    envoy_ext_proc_v3.ProcessingMode_STREAMED,
			ResponseTrailerMode: envoy_ext_proc_v3.ProcessingMode_SKIP,
		},
		MessageTimeout: durationpb.New(5 * time.Second),
		MetadataOptions: &envoy_ext_proc_v3.MetadataOptions{
			ForwardingNamespaces: &envoy_ext_proc_v3.MetadataOptions_MetadataNamespaces{
				Untyped: []string{"io.solo.transformation", "envoy.filters.ai.solo.io"},
				Typed:   []string{"envoy.filters.ai.solo.io"},
			},
			ReceivingNamespaces: &envoy_ext_proc_v3.MetadataOptions_MetadataNamespaces{
				Untyped: []string{"ai.kgateway.io"},
			},
		},
	}
	// Run before rate limiting
	stagedFilter, err := plugins.NewStagedFilter(
		wellknown.AIExtProcFilterName,
		extProcSettings,
		plugins.FilterStage[plugins.WellKnownFilterStage]{
			RelativeTo: plugins.RateLimitStage,
			Weight:     -2,
		},
	)
	if err != nil {
		return nil, err
	}
	result = append(result, stagedFilter)
	return result, nil
}
