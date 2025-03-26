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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// AddExtProcHTTPFilter adds an extproc filter to the http filter chain
func AddExtProcHTTPFilter(extProcConfig *envoy_ext_proc_v3.ExternalProcessor) ([]plugins.StagedHttpFilter, error) {
	// needed?
	// if err := extProcConfig.ValidateAll(); err != nil {
	// 	return nil, err
	// }
	// extprocAny, err := utils.MessageToAny(extProcConfig)
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

func enableExtprocFilter(pCtx *ir.RouteBackendContext) {
	cfg := &routev3.FilterConfig{
		Config: &anypb.Any{},
	}

	pCtx.TypedFilterConfig.AddTypedConfig(wellknown.ExtprocFilterName, cfg)
}

// toEnvoyExtProc converts an ExtProcPolicy to an ExternalProcessor
func toEnvoyExtProc(
	extprocConfig *v1alpha1.ExtProcPolicy,
	krtctx krt.HandlerContext,
	commoncol *common.CommonCollections,
	parentSrc ir.ObjectSource,
) (*envoy_ext_proc_v3.ExternalProcessor, error) {
	backend, err := commoncol.BackendIndex.GetBackendFromRef(krtctx, parentSrc, extprocConfig.GrpcService.BackendRef.BackendObjectReference)
	if err != nil {
		// return nil, err
		fmt.Println("error getting backend", err)
		return nil, err
	}
	envoyGrpcService := &envoy_config_core_v3.GrpcService{
		TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
				ClusterName: backend.ClusterName(),
			},
		},
	}
	if extprocConfig.GrpcService.Authority != nil {
		envoyGrpcService.GetEnvoyGrpc().Authority = *extprocConfig.GrpcService.Authority
	}

	envoyExtProc := &envoy_ext_proc_v3.ExternalProcessor{
		GrpcService: envoyGrpcService,
	}

	if extprocConfig.ProcessingMode != nil {
		envoyExtProc.ProcessingMode = ToEnvoyProcessingMode(extprocConfig.ProcessingMode)
	}

	// filter metadata?
	// failure mode?
	// what else to add to config?
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

// ToEnvoyProcessingMode converts our ProcessingMode to envoy's ProcessingMode
func ToEnvoyProcessingMode(p *v1alpha1.ProcessingMode) *envoy_ext_proc_v3.ProcessingMode {
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
