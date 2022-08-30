package api_conversion

import (
	envoytrace "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	envoytrace_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/trace/v3"
)

// Converts between Envoy and Gloo/solokit versions of envoy protos
// This is required because go-control-plane dropped gogoproto in favor of goproto
// in v0.9.0, but solokit depends on gogoproto (and the generated deep equals it creates).
//
// we should work to remove that assumption from solokit and delete this code:
// https://github.com/solo-io/gloo/issues/1793

func ToEnvoyDatadogConfiguration(glooDatadogConfig *envoytrace_gloo.DatadogConfig, clusterName string) (*envoytrace.DatadogConfig, error) {
	envoyDatadogConfig := &envoytrace.DatadogConfig{
		CollectorCluster: clusterName,
		ServiceName:      glooDatadogConfig.GetServiceName().GetValue(),
	}
	return envoyDatadogConfig, nil
}

func ToEnvoyZipkinConfiguration(glooZipkinConfig *envoytrace_gloo.ZipkinConfig, clusterName string) (*envoytrace.ZipkinConfig, error) {
	envoyZipkinConfig := &envoytrace.ZipkinConfig{
		CollectorCluster:         clusterName,
		CollectorEndpoint:        glooZipkinConfig.GetCollectorEndpoint(),
		CollectorEndpointVersion: ToEnvoyZipkinCollectorEndpointVersion(glooZipkinConfig.GetCollectorEndpointVersion()),
		TraceId_128Bit:           glooZipkinConfig.GetTraceId_128Bit().GetValue(),
		SharedSpanContext:        glooZipkinConfig.GetSharedSpanContext(),
	}
	return envoyZipkinConfig, nil
}

func ToEnvoyZipkinCollectorEndpointVersion(version envoytrace_gloo.ZipkinConfig_CollectorEndpointVersion) envoytrace.ZipkinConfig_CollectorEndpointVersion {
	switch str := version.String(); str {
	case envoytrace_gloo.ZipkinConfig_CollectorEndpointVersion_name[int32(envoytrace_gloo.ZipkinConfig_HTTP_JSON)]:
		return envoytrace.ZipkinConfig_HTTP_JSON
	case envoytrace_gloo.ZipkinConfig_CollectorEndpointVersion_name[int32(envoytrace_gloo.ZipkinConfig_HTTP_PROTO)]:
		return envoytrace.ZipkinConfig_HTTP_PROTO
	}
	return envoytrace.ZipkinConfig_HTTP_JSON
}
