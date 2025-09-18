package ai

import (
	"fmt"
	"strings"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	envoy_upstream_codec "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/upstream_codec/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	upstream_wait "github.com/solo-io/envoy-gloo/go/config/filter/http/upstream_wait/v2"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	translatorutils "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

const (
	// upstreamCodecFilterName is the name of the upstream codec filter.
	upstreamCodecFilterName = "envoy.filters.http.upstream_codec"
)

func AddUpstreamClusterHttpFilters(out *envoyclusterv3.Cluster) error {
	transformationMsg, err := utils.MessageToAny(&envoytransformation.FilterTransformations{})
	if err != nil {
		return err
	}

	upstreamWaitMsg, err := utils.MessageToAny(&upstream_wait.UpstreamWaitFilterConfig{})
	if err != nil {
		return err
	}

	codecConfigAny, err := utils.MessageToAny(&envoy_upstream_codec.UpstreamCodec{})
	if err != nil {
		return fmt.Errorf("failed to create upstream codec config: %v", err)
	}

	// The order of the filters is important as AIPolicyTransformationFilterName must run before the AIBackendTransformationFilterName
	orderedFilters := []*envoy_hcm.HttpFilter{
		// The wait filter essentially blocks filter iteration until a host has been selected.
		// This is important because running as an upstream filter allows access to host
		// metadata iff the host has already been selected, and that's a
		// major benefit of running the filter at this stage.
		{
			Name: waitFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
				TypedConfig: upstreamWaitMsg,
			},
		},
		{
			Name: wellknown.AIPolicyTransformationFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
				TypedConfig: transformationMsg,
			},
		},
		{
			Name: wellknown.AIBackendTransformationFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
				TypedConfig: transformationMsg,
			},
		},
		{
			Name: upstreamCodecFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
				TypedConfig: codecConfigAny,
			},
		},
	}

	if err = translatorutils.MutateHttpOptions(out, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
		ts := out.GetTransportSocket()
		supportsALPN := false
		if ts != nil {
			tc := ts.GetTypedConfig()
			if tc != nil && (strings.Contains(tc.GetTypeUrl(), "UpstreamTlsContext") || strings.Contains(tc.GetTypeUrl(), "QuicUpstreamTransport")) {
				// if upstream supports ALPN, add the auto config
				supportsALPN = true
			}
		}
		if supportsALPN {
			opts.UpstreamProtocolOptions = &envoy_upstreams_v3.HttpProtocolOptions_AutoConfig{
				AutoConfig: &envoy_upstreams_v3.HttpProtocolOptions_AutoHttpConfig{},
			}
		} else {
			// otherwise we use http1 for upstream that do not support ALPN
			opts.UpstreamProtocolOptions = &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
				},
			}
		}
		opts.CommonHttpProtocolOptions = &envoycorev3.HttpProtocolOptions{
			IdleTimeout: &durationpb.Duration{
				Seconds: 30,
			},
		}
		opts.HttpFilters = append(opts.GetHttpFilters(), orderedFilters...)
	}); err != nil {
		return err
	}

	return nil
}

func AddExtprocHTTPFilter() ([]plugins.StagedHttpFilter, error) {
	var result []plugins.StagedHttpFilter

	// TODO: add ratelimit and jwt_authn if AI Backend is configured

	extProcSettings := &envoy_ext_proc_v3.ExternalProcessor{
		GrpcService: &envoycorev3.GrpcService{
			// Note: retries and timeouts are not set here currently since grpc retries are not useful if the
			// request size is unknown. See: https://github.com/kgateway-dev/kgateway/issues/10739
			TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
					ClusterName: extProcUDSClusterName,
				},
			},
		},
		ProcessingMode: &envoy_ext_proc_v3.ProcessingMode{
			RequestHeaderMode: envoy_ext_proc_v3.ProcessingMode_SEND,
			// TODO: Change this to buffered. Set limit by add buffer filter for AI requests, disabled by default
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
		plugins.HTTPFilterStage{
			RelativeTo:     plugins.RateLimitStage,
			RelativeWeight: -2,
		},
	)
	if err != nil {
		return nil, err
	}
	result = append(result, stagedFilter)
	return result, nil
}
