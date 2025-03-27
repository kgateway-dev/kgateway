package routepolicy

import (
	"fmt"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// addExtProcHTTPFilter adds an extproc filter to the http filter chain
func addExtProcHTTPFilter(extProcConfig *envoy_ext_proc_v3.ExternalProcessor) ([]plugins.StagedHttpFilter, error) {
	extprocFilter, err := plugins.NewStagedFilter(
		wellknown.ExtprocFilterName,
		extProcConfig,
		plugins.AfterStage(plugins.WellKnownFilterStage(plugins.AuthZStage)),
	)
	// disable the filter by default
	extprocFilter.Filter.Disabled = true
	if err != nil {
		return nil, err
	}
	return []plugins.StagedHttpFilter{extprocFilter}, nil
}

func enableExtprocFilterPerRoute(pCtx *ir.RouteContext) {
	cfg := &routev3.FilterConfig{
		Config: &anypb.Any{},
	}

	pCtx.TypedFilterConfig.AddTypedConfig(wellknown.ExtprocFilterName, cfg)
}

func enableExtprocFilter(pCtx *ir.RouteBackendContext) {
	cfg := &routev3.FilterConfig{
		Config: &anypb.Any{},
	}

	pCtx.TypedFilterConfig.AddTypedConfig(wellknown.ExtprocFilterName, cfg)
}

// toEnvoyExtProc converts an ExtProcPolicy to an ExternalProcessor
func toEnvoyExtProc(
	trafficPolicy *v1alpha1.TrafficPolicy,
	krtctx krt.HandlerContext,
	commoncol *common.CommonCollections,
) (*envoy_ext_proc_v3.ExternalProcessor, error) {
	extprocConfig := trafficPolicy.Spec.ExtProc
	gExt, err := pluginutils.GetGatewayExtension(commoncol.GatewayExtensions, krtctx, extprocConfig.ExtensionRef.Name, trafficPolicy.GetNamespace())
	if err != nil {
		return nil, fmt.Errorf("failed to get GatewayExtension %s: %s", extprocConfig.ExtensionRef.Name, err.Error())
	}
	backend, err := commoncol.BackendIndex.GetBackendFromRef(krtctx, gExt.ObjectSource, gExt.ExtProc.GrpcService.BackendRef.BackendObjectReference)
	// TODO: what is the correct behavior? maybe route to static blackhole?
	if err != nil {
		return nil, fmt.Errorf("failed to get backend from GatewayExtension %s: %s", gExt.ObjectSource.GetName(), err.Error())
	}
	envoyGrpcService := &envoy_config_core_v3.GrpcService{
		TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
				ClusterName: backend.ClusterName(),
			},
		},
	}
	if gExt.ExtProc.GrpcService.Authority != nil {
		envoyGrpcService.GetEnvoyGrpc().Authority = *gExt.ExtProc.GrpcService.Authority
	}

	envoyExtProc := &envoy_ext_proc_v3.ExternalProcessor{
		GrpcService: envoyGrpcService,
	}

	if extprocConfig.ProcessingMode != nil {
		envoyExtProc.ProcessingMode = toEnvoyProcessingMode(extprocConfig.ProcessingMode)
	}

	if extprocConfig.FailureModeAllow != nil {
		envoyExtProc.FailureModeAllow = *extprocConfig.FailureModeAllow
	}

	return envoyExtProc, nil
}

// headerSendModeFromString converts a string to envoy HeaderSendMode
func headerSendModeFromString(mode *string) envoy_ext_proc_v3.ProcessingMode_HeaderSendMode {
	if mode == nil {
		return envoy_ext_proc_v3.ProcessingMode_DEFAULT
	}
	switch *mode {
	case "SEND":
		return envoy_ext_proc_v3.ProcessingMode_SEND
	case "SKIP":
		return envoy_ext_proc_v3.ProcessingMode_SKIP
	default:
		return envoy_ext_proc_v3.ProcessingMode_DEFAULT
	}
}

// bodySendModeFromString converts a string to envoy BodySendMode
func bodySendModeFromString(mode *string) envoy_ext_proc_v3.ProcessingMode_BodySendMode {
	if mode == nil {
		return envoy_ext_proc_v3.ProcessingMode_NONE
	}
	switch *mode {
	case "STREAMED":
		return envoy_ext_proc_v3.ProcessingMode_STREAMED
	case "BUFFERED":
		return envoy_ext_proc_v3.ProcessingMode_BUFFERED
	case "BUFFERED_PARTIAL":
		return envoy_ext_proc_v3.ProcessingMode_BUFFERED_PARTIAL
	case "FULL_DUPLEX_STREAMED":
		return envoy_ext_proc_v3.ProcessingMode_FULL_DUPLEX_STREAMED
	default:
		return envoy_ext_proc_v3.ProcessingMode_NONE
	}
}

// toEnvoyProcessingMode converts our ProcessingMode to envoy's ProcessingMode
func toEnvoyProcessingMode(p *v1alpha1.ProcessingMode) *envoy_ext_proc_v3.ProcessingMode {
	if p == nil {
		return nil
	}

	return &envoy_ext_proc_v3.ProcessingMode{
		RequestHeaderMode:   headerSendModeFromString(p.RequestHeaderMode),
		ResponseHeaderMode:  headerSendModeFromString(p.ResponseHeaderMode),
		RequestBodyMode:     bodySendModeFromString(p.RequestBodyMode),
		ResponseBodyMode:    bodySendModeFromString(p.ResponseBodyMode),
		RequestTrailerMode:  headerSendModeFromString(p.RequestTrailerMode),
		ResponseTrailerMode: headerSendModeFromString(p.ResponseTrailerMode),
	}
}
