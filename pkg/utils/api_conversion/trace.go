package api_conversion

import (
	v1 "github.com/census-instrumentation/opencensus-proto/gen-go/trace/v1"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytrace "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	envoytracegloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/trace/v3"
)

// Converts between Envoy and Gloo/solokit versions of envoy protos
// This is required because go-control-plane dropped gogoproto in favor of goproto
// in v0.9.0, but solokit depends on gogoproto (and the generated deep equals it creates).
//
// we should work to remove that assumption from solokit and delete this code:
// https://github.com/solo-io/gloo/issues/1793

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

func ToEnvoyOpenTelemetryonfiguration(glooOpenTelemetryConfig *envoytracegloo.OpenTelemetryConfig, clusterName string) (*envoytrace.OpenTelemetryConfig, error) {
	envoyOpenTelemetryConfig := &envoytrace.OpenTelemetryConfig{
		GrpcService: &envoy_config_core_v3.GrpcService{
			TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
					ClusterName: clusterName,
				},
			},
		},
	}

	return envoyOpenTelemetryConfig, nil

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
