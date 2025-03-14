package routepolicy

import (
	"fmt"
	"time"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

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

func AddExtprocHTTPFilter() ([]plugins.StagedHttpFilter, error) {
	extprocFilter, err := plugins.NewStagedFilter(
		wellknown.ExtprocFilterName,
		&envoy_ext_proc_v3.ExternalProcessor{
			GrpcService: &envoy_config_core_v3.GrpcService{
				TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
						ClusterName: "kube_default_ext-proc-grpc_4444", //"kube_default_grpc-ext-proc_9002", //"blackhole22",
						Authority:   "ext-proc-grpc.default:4444",      //"grpc-ext-proc.default:9002",
					},
				},
				Timeout: durationpb.New(10 * time.Second),
			},
			// FailureModeAllow: true,
			ProcessingMode: &envoy_ext_proc_v3.ProcessingMode{
				RequestHeaderMode:  envoy_ext_proc_v3.ProcessingMode_SEND,
				ResponseHeaderMode: envoy_ext_proc_v3.ProcessingMode_SEND,
				// RequestBodyMode:     envoy_ext_proc_v3.ProcessingMode_STREAMED,
				ResponseBodyMode:    envoy_ext_proc_v3.ProcessingMode_STREAMED,
				RequestTrailerMode:  envoy_ext_proc_v3.ProcessingMode_SKIP,
				ResponseTrailerMode: envoy_ext_proc_v3.ProcessingMode_SKIP,
			},
		},
		plugins.AfterStage(plugins.WellKnownFilterStage(plugins.AuthZStage)),
	)
	if err != nil {
		return nil, err
	}
	return []plugins.StagedHttpFilter{extprocFilter}, nil
}

func toEnvoyExtProcPerRoute(
	extprocConfig *v1alpha1.ExtProcPolicy,
	krtctx krt.HandlerContext,
	commoncol *common.CommonCollections,
	parentSrc ir.ObjectSource,
) (*envoy_ext_proc_v3.ExtProcPerRoute, error) {
	backend, err := commoncol.Backends.GetBackendFromRef(krtctx, parentSrc, extprocConfig.GrpcService.BackendRef.BackendObjectReference)
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

	extProcPerRoute := &envoy_ext_proc_v3.ExtProcOverrides{
		GrpcService: envoyGrpcService,
	}

	if extprocConfig.ProcessingMode != nil {
		extProcPerRoute.ProcessingMode = ToEnvoyProcessingMode(extprocConfig.ProcessingMode)
	}

	return &envoy_ext_proc_v3.ExtProcPerRoute{
		Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
			Overrides: extProcPerRoute,
		},
	}, nil
}

// func enableExtprocFilter(pCtx *ir.RouteContext) {
// 	extProc := &

// }

func enableExtprocFilter(pCtx *ir.RouteBackendContext) {
	cfg := &routev3.FilterConfig{
		Config: &anypb.Any{},
	}

	pCtx.AddTypedConfig(wellknown.ExtprocFilterName, cfg)
}

func ExtprocHCMFilter(extprocIR *ExtprocIR) (*hcmv3.HttpFilter, error) {
	if err := extprocIR.ExtProc.ValidateAll(); err != nil {
		return nil, err
	}
	extprocAny, err := utils.MessageToAny(extprocIR.ExtProc)
	if err != nil {
		return nil, err
	}

	return &hcmv3.HttpFilter{
		Name: wellknown.ExtprocFilterName,
		ConfigType: &hcmv3.HttpFilter_TypedConfig{
			TypedConfig: extprocAny,
		},
		Disabled: true,
	}, nil
}

func filterName(name string) string {
	return fmt.Sprintf("%s/%s", wellknown.ExtprocFilterName, name) // dont need
}

// toEnvoyExtProc converts an ExtProcPolicy to an ExternalProcessor
func toEnvoyExtProc(
	extprocConfig *v1alpha1.ExtProcPolicy,
	krtctx krt.HandlerContext,
	commoncol *common.CommonCollections,
	parentSrc ir.ObjectSource,
) (*envoy_ext_proc_v3.ExternalProcessor, error) {
	backend, err := commoncol.Backends.GetBackendFromRef(krtctx, parentSrc, extprocConfig.GrpcService.BackendRef.BackendObjectReference)
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

// HeaderSendModeFromString converts a string to envoy HeaderSendMode
func HeaderSendModeFromString(mode *string) envoy_ext_proc_v3.ProcessingMode_HeaderSendMode {
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

// BodySendModeFromString converts a string to envoy BodySendMode
func BodySendModeFromString(mode *string) envoy_ext_proc_v3.ProcessingMode_BodySendMode {
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
		RequestHeaderMode:   HeaderSendModeFromString(p.RequestHeaderMode),
		ResponseHeaderMode:  HeaderSendModeFromString(p.ResponseHeaderMode),
		RequestBodyMode:     BodySendModeFromString(p.RequestBodyMode),
		ResponseBodyMode:    BodySendModeFromString(p.ResponseBodyMode),
		RequestTrailerMode:  HeaderSendModeFromString(p.RequestTrailerMode),
		ResponseTrailerMode: HeaderSendModeFromString(p.ResponseTrailerMode),
	}
}
