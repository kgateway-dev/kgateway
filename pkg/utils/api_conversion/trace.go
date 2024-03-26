package api_conversion

import (
	"context"
	"strings"

	v1 "github.com/census-instrumentation/opencensus-proto/gen-go/trace/v1"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytrace "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	envoytracegloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/trace/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"go.uber.org/zap"
)

// Converts between Envoy and Gloo/solokit versions of envoy protos

const (
	DeprecatedMetadataServiceName = "deprecated_metadata"
	UndefinedMetadataServiceName  = "undefined_gateway"
	UnkownMetadataServiceName     = "unknown_metadata"
)

var (
	gatewayKinds = gatewayKindsMap()
)

func ToEnvoyDatadogConfiguration(glooDatadogConfig *envoytracegloo.DatadogConfig, clusterName string) (*envoytrace.DatadogConfig, error) {
	envoyDatadogConfig := &envoytrace.DatadogConfig{
		CollectorCluster: clusterName,
		ServiceName:      glooDatadogConfig.GetServiceName().GetValue(),
	}
	return envoyDatadogConfig, nil
}

func ToEnvoyZipkinConfiguration(glooZipkinConfig *envoytracegloo.ZipkinConfig, clusterName string) (*envoytrace.ZipkinConfig, error) {
	envoyZipkinConfig := &envoytrace.ZipkinConfig{
		CollectorCluster:         clusterName,
		CollectorEndpoint:        glooZipkinConfig.GetCollectorEndpoint(),
		CollectorEndpointVersion: ToEnvoyZipkinCollectorEndpointVersion(glooZipkinConfig.GetCollectorEndpointVersion()),
		TraceId_128Bit:           glooZipkinConfig.GetTraceId_128Bit().GetValue(),
		SharedSpanContext:        glooZipkinConfig.GetSharedSpanContext(),
	}
	return envoyZipkinConfig, nil
}

// GetGatewayNameFromParent returns the name of the gateway that the listener is associated with.
// This is used by the otel plugin to set the service name. It requires that the gateway populate the listener's
// SourceMetadata with the gateway's data. The resource_kind field is a string, and different gateways may use different
// strings to represent their kind. In the case of unexpected metadata format (eg, no gateways or multiple gateways), we
// will log a warning and return a service.name string to help identify the issue.
func GetGatewayNameFromParent(ctx context.Context, parent *gloov1.Listener) string {

	switch metadata := parent.GetOpaqueMetadata().(type) {
	// Deprecated metadata format. This is not an expected condition, as it is the control plane's responsibility to set this field,
	// and it no longer uses the deprecated API
	case *gloov1.Listener_Metadata:
		contextutils.LoggerFrom(ctx).Warn("Using deprecated 'Metadata' format for gateway name in parent listener metadata. Please update your gateway to use the new format")
		return DeprecatedMetadataServiceName
	// Expected/desired metadata format
	case *gloov1.Listener_MetadataStatic:
		gateways := []string{}
		for _, source := range metadata.MetadataStatic.GetSources() {
			if isResourceGateway(source) {
				gateways = append(gateways, source.GetResourceRef().GetName())
			}
		}
		switch {
		// No gateways found - this is unexpected
		case len(gateways) == 0:
			contextutils.LoggerFrom(ctx).Warn("No gateway found in parent listener metadata")
			return UndefinedMetadataServiceName
		// Mulitple gateways found. This is unexpected, as there should only be one gateway
		case len(gateways) > 1:
			contextutils.LoggerFrom(ctx).Warnw("Multiple gateways found in listener metadata", zap.Strings("gateways", gateways))
			return strings.Join(gateways, ",")
		default: // exactly 1, what we expect
			return gateways[0]
		}
	default:
		// if we reach this error its most likely because the API was updated with a new type but this code wasn't
		// or the fucntion was called with a nil listener
		contextutils.LoggerFrom(ctx).Warn("Unknown listener metadata format")
		return UnkownMetadataServiceName
	}
}

// isResourceGateway returns true if the resource is a gateway
// This logic is split out to easily manage it as we add more gateway types.
// If this function is updated, update this documentation we well: https://github.com/solo-io/gloo/tree/main/devel/architecture/static_metadata.md
func isResourceGateway(resource *gloov1.SourceMetadata_SourceRef) bool {
	_, ok := gatewayKinds[resource.GetResourceKind()]

	return ok
}

func gatewayKindsMap() map[string]bool {
	return map[string]bool{
		resources.Kind(new(gatewayv1.Gateway)): true,
	}
}

// ToEnvoyOpenTelemetryConfiguration converts a Gloo OpenTelemetryConfig to an Envoy OpenTelemetryConfig
func ToEnvoyOpenTelemetryConfiguration(clusterName, serviceName string) *envoytrace.OpenTelemetryConfig {
	envoyOpenTelemetryConfig := &envoytrace.OpenTelemetryConfig{
		GrpcService: &envoy_config_core_v3.GrpcService{
			TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
					ClusterName: clusterName,
				},
			},
		},
		ServiceName: serviceName,
	}

	return envoyOpenTelemetryConfig
}

func ToEnvoyOpenCensusConfiguration(glooOpenCensusConfig *envoytracegloo.OpenCensusConfig) (*envoytrace.OpenCensusConfig, error) {

	envoyOpenCensusConfig := &envoytrace.OpenCensusConfig{
		TraceConfig: &v1.TraceConfig{
			Sampler:                  nil,
			MaxNumberOfAttributes:    glooOpenCensusConfig.GetTraceConfig().GetMaxNumberOfAttributes(),
			MaxNumberOfAnnotations:   glooOpenCensusConfig.GetTraceConfig().GetMaxNumberOfAnnotations(),
			MaxNumberOfMessageEvents: glooOpenCensusConfig.GetTraceConfig().GetMaxNumberOfMessageEvents(),
			MaxNumberOfLinks:         glooOpenCensusConfig.GetTraceConfig().GetMaxNumberOfLinks(),
		},
		OcagentExporterEnabled: glooOpenCensusConfig.GetOcagentExporterEnabled(),
		IncomingTraceContext:   translateTraceContext(glooOpenCensusConfig.GetIncomingTraceContext()),
		OutgoingTraceContext:   translateTraceContext(glooOpenCensusConfig.GetOutgoingTraceContext()),
	}

	switch glooOpenCensusConfig.GetOcagentAddress().(type) {
	case *envoytracegloo.OpenCensusConfig_HttpAddress:
		envoyOpenCensusConfig.OcagentAddress = glooOpenCensusConfig.GetHttpAddress()
	case *envoytracegloo.OpenCensusConfig_GrpcAddress:
		grpcAddress := glooOpenCensusConfig.GetGrpcAddress()
		envoyOpenCensusConfig.OcagentGrpcService = &envoy_config_core_v3.GrpcService{
			TargetSpecifier: &envoy_config_core_v3.GrpcService_GoogleGrpc_{
				GoogleGrpc: &envoy_config_core_v3.GrpcService_GoogleGrpc{
					TargetUri:  grpcAddress.GetTargetUri(),
					StatPrefix: grpcAddress.GetStatPrefix(),
				},
			},
		}
	}

	translateTraceConfig(glooOpenCensusConfig.GetTraceConfig(), envoyOpenCensusConfig.GetTraceConfig())

	return envoyOpenCensusConfig, nil
}

func translateTraceConfig(glooTraceConfig *envoytracegloo.TraceConfig, envoyTraceConfig *v1.TraceConfig) {
	switch glooTraceConfig.GetSampler().(type) {
	case *envoytracegloo.TraceConfig_ConstantSampler:
		var decision v1.ConstantSampler_ConstantDecision
		switch glooTraceConfig.GetConstantSampler().GetDecision() {
		case envoytracegloo.ConstantSampler_ALWAYS_ON:
			decision = v1.ConstantSampler_ALWAYS_ON
		case envoytracegloo.ConstantSampler_ALWAYS_OFF:
			decision = v1.ConstantSampler_ALWAYS_OFF
		case envoytracegloo.ConstantSampler_ALWAYS_PARENT:
			decision = v1.ConstantSampler_ALWAYS_PARENT
		}
		envoyTraceConfig.Sampler = &v1.TraceConfig_ConstantSampler{
			ConstantSampler: &v1.ConstantSampler{
				Decision: decision,
			},
		}
	case *envoytracegloo.TraceConfig_ProbabilitySampler:
		envoyTraceConfig.Sampler = &v1.TraceConfig_ProbabilitySampler{
			ProbabilitySampler: &v1.ProbabilitySampler{
				SamplingProbability: glooTraceConfig.GetProbabilitySampler().GetSamplingProbability(),
			},
		}
	case *envoytracegloo.TraceConfig_RateLimitingSampler:
		envoyTraceConfig.Sampler = &v1.TraceConfig_RateLimitingSampler{RateLimitingSampler: &v1.RateLimitingSampler{
			Qps: glooTraceConfig.GetRateLimitingSampler().GetQps(),
		}}
	}
}

func translateTraceContext(glooTraceContexts []envoytracegloo.OpenCensusConfig_TraceContext) []envoytrace.OpenCensusConfig_TraceContext {
	result := make([]envoytrace.OpenCensusConfig_TraceContext, 0, len(glooTraceContexts))
	for _, glooTraceContext := range glooTraceContexts {
		var envoyTraceContext envoytrace.OpenCensusConfig_TraceContext
		switch glooTraceContext {
		case envoytracegloo.OpenCensusConfig_NONE:
			envoyTraceContext = envoytrace.OpenCensusConfig_NONE
		case envoytracegloo.OpenCensusConfig_TRACE_CONTEXT:
			envoyTraceContext = envoytrace.OpenCensusConfig_TRACE_CONTEXT
		case envoytracegloo.OpenCensusConfig_GRPC_TRACE_BIN:
			envoyTraceContext = envoytrace.OpenCensusConfig_GRPC_TRACE_BIN
		case envoytracegloo.OpenCensusConfig_CLOUD_TRACE_CONTEXT:
			envoyTraceContext = envoytrace.OpenCensusConfig_CLOUD_TRACE_CONTEXT
		case envoytracegloo.OpenCensusConfig_B3:
			envoyTraceContext = envoytrace.OpenCensusConfig_B3
		}
		result = append(result, envoyTraceContext)
	}
	return result
}

func ToEnvoyZipkinCollectorEndpointVersion(version envoytracegloo.ZipkinConfig_CollectorEndpointVersion) envoytrace.ZipkinConfig_CollectorEndpointVersion {
	switch str := version.String(); str {
	case envoytracegloo.ZipkinConfig_CollectorEndpointVersion_name[int32(envoytracegloo.ZipkinConfig_HTTP_JSON)]:
		return envoytrace.ZipkinConfig_HTTP_JSON
	case envoytracegloo.ZipkinConfig_CollectorEndpointVersion_name[int32(envoytracegloo.ZipkinConfig_HTTP_PROTO)]:
		return envoytrace.ZipkinConfig_HTTP_PROTO
	}
	return envoytrace.ZipkinConfig_HTTP_JSON
}
