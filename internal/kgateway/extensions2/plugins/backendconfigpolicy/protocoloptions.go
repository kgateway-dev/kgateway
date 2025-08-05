package backendconfigpolicy

import (
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	preserve_case_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/header_formatters/preserve_case/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	translatorutils "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

func translateCommonHttpProtocolOptions(commonHttpProtocolOptions *v1alpha1.CommonHttpProtocolOptions) *envoycorev3.HttpProtocolOptions {
	out := &envoycorev3.HttpProtocolOptions{}
	if commonHttpProtocolOptions.MaxRequestsPerConnection != nil {
		out.MaxRequestsPerConnection = &wrapperspb.UInt32Value{Value: uint32(*commonHttpProtocolOptions.MaxRequestsPerConnection)}
	}
	if commonHttpProtocolOptions.IdleTimeout != nil {
		out.IdleTimeout = durationpb.New(commonHttpProtocolOptions.IdleTimeout.Duration)
	}

	if commonHttpProtocolOptions.MaxHeadersCount != nil {
		out.MaxHeadersCount = &wrapperspb.UInt32Value{Value: uint32(*commonHttpProtocolOptions.MaxHeadersCount)}
	}

	if commonHttpProtocolOptions.MaxStreamDuration != nil {
		out.MaxStreamDuration = durationpb.New(commonHttpProtocolOptions.MaxStreamDuration.Duration)
	}

	return out
}

func translateHttp1ProtocolOptions(http1ProtocolOptions *v1alpha1.Http1ProtocolOptions) (*envoycorev3.Http1ProtocolOptions, error) {
	out := &envoycorev3.Http1ProtocolOptions{}
	if http1ProtocolOptions.EnableTrailers != nil {
		out.EnableTrailers = *http1ProtocolOptions.EnableTrailers
	}

	if http1ProtocolOptions.PreserveHttp1HeaderCase != nil && *http1ProtocolOptions.PreserveHttp1HeaderCase {
		typedConfig, err := utils.MessageToAny(&preserve_case_v3.PreserveCaseFormatterConfig{})
		if err != nil {
			return nil, err
		}
		out.HeaderKeyFormat = &envoycorev3.Http1ProtocolOptions_HeaderKeyFormat{
			HeaderFormat: &envoycorev3.Http1ProtocolOptions_HeaderKeyFormat_StatefulFormatter{
				StatefulFormatter: &envoycorev3.TypedExtensionConfig{
					Name:        PreserveCasePlugin,
					TypedConfig: typedConfig,
				},
			},
		}
	}

	if http1ProtocolOptions.OverrideStreamErrorOnInvalidHttpMessage != nil {
		out.OverrideStreamErrorOnInvalidHttpMessage = &wrapperspb.BoolValue{Value: *http1ProtocolOptions.OverrideStreamErrorOnInvalidHttpMessage}
	}

	return out, nil
}

func translateHttp2ProtocolOptions(http2ProtocolOptions *v1alpha1.Http2ProtocolOptions) *envoycorev3.Http2ProtocolOptions {
	out := &envoycorev3.Http2ProtocolOptions{}
	if http2ProtocolOptions.MaxConcurrentStreams != nil {
		out.MaxConcurrentStreams = &wrapperspb.UInt32Value{Value: uint32(*http2ProtocolOptions.MaxConcurrentStreams)}
	}
	if http2ProtocolOptions.InitialStreamWindowSize != nil {
		out.InitialStreamWindowSize = &wrapperspb.UInt32Value{Value: uint32(http2ProtocolOptions.InitialStreamWindowSize.Value())}
	}
	if http2ProtocolOptions.InitialConnectionWindowSize != nil {
		out.InitialConnectionWindowSize = &wrapperspb.UInt32Value{Value: uint32(http2ProtocolOptions.InitialConnectionWindowSize.Value())}
	}
	if http2ProtocolOptions.OverrideStreamErrorOnInvalidHttpMessage != nil {
		out.OverrideStreamErrorOnInvalidHttpMessage = &wrapperspb.BoolValue{Value: *http2ProtocolOptions.OverrideStreamErrorOnInvalidHttpMessage}
	}
	return out
}

func applyCommonHttpProtocolOptions(commonHttpProtocolOptions *envoycorev3.HttpProtocolOptions, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
	if commonHttpProtocolOptions == nil {
		return
	}

	if err := translatorutils.MutateHttpOptions(out, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
		opts.CommonHttpProtocolOptions = commonHttpProtocolOptions
		if opts.GetUpstreamProtocolOptions() == nil {
			// Envoy requires UpstreamProtocolOptions if CommonHttpProtocolOptions is set.
			opts.UpstreamProtocolOptions = &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
				},
			}
		}
	}); err != nil {
		logger.Error("failed to apply common http protocol options", "error", err)
	}
}

func applyHttp1ProtocolOptions(http1ProtocolOptions *envoycorev3.Http1ProtocolOptions, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
	if http1ProtocolOptions == nil {
		return
	}

	if backend.AppProtocol == ir.HTTP2AppProtocol {
		logger.Warn("can't apply http1 protocol options to http2 backend", "backend", backend.GetName())
		return
	}

	if err := translatorutils.MutateHttpOptions(out, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
		opts.UpstreamProtocolOptions = &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{
					HttpProtocolOptions: http1ProtocolOptions,
				},
			},
		}
	}); err != nil {
		logger.Error("failed to apply http1 protocol options", "backend", backend.GetName(), "error", err)
	}
}

func applyHttp2ProtocolOptions(http2ProtocolOptions *envoycorev3.Http2ProtocolOptions, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
	if http2ProtocolOptions == nil {
		return
	}

	if backend.AppProtocol != ir.HTTP2AppProtocol {
		logger.Warn("can't apply http2 protocol options to non-http2 backend", "backend", backend.GetName())
		return
	}

	if err := translatorutils.MutateHttpOptions(out, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
		opts.UpstreamProtocolOptions = &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: http2ProtocolOptions,
				},
			},
		}
	}); err != nil {
		logger.Error("failed to apply http2 protocol options", "backend", backend.GetName(), "error", err)
	}
}
