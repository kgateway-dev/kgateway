package gogoutils

import (
	envoytrace "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	envoytrace_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/trace/v3"
)

// Converts between Envoy and Gloo/solokit versions of envoy protos
// This is required because go-control-plane dropped gogoproto in favor of goproto
// in v0.9.0, but solokit depends on gogoproto (and the generated deep equals it creates).
//
// we should work to remove that assumption from solokit and delete this code:
// https://github.com/solo-io/gloo/issues/1793

func ToGlooHttpTracingProvider(envoyHttpTracingProvider *envoytrace.Tracing_Http) (*envoytrace_gloo.Tracing_Http, error) {
	if envoyHttpTracingProvider == nil {
		return nil, nil
	}

	glooTracingProvider := &envoytrace_gloo.Tracing_Http{
		Name: envoyHttpTracingProvider.GetName(),
	}

	switch typedConfig := envoyHttpTracingProvider.GetConfigType().(type) {
	case *envoytrace.Tracing_Http_TypedConfig:
		converted, err := protoutils.AnyPbToGogo(typedConfig.TypedConfig)
		if err != nil {
			return nil, err
		}
		glooTracingProvider.ConfigType = &envoytrace_gloo.Tracing_Http_TypedConfig{
			TypedConfig: converted,
		}
	}

	return glooTracingProvider, nil
}


func ToEnvoyHttpTracingProvider(glooTracingProvider *envoytrace_gloo.Tracing_Http) (*envoytrace.Tracing_Http, error) {
	if glooTracingProvider == nil {
		return nil, nil
	}

	envoyHttpTracingProvider := &envoytrace.Tracing_Http{
		Name: glooTracingProvider.GetName(),
	}

	switch typedConfig := glooTracingProvider.GetConfigType().(type) {
	case *envoytrace_gloo.Tracing_Http_TypedConfig:
		converted, err := protoutils.AnyGogoToPb(typedConfig.TypedConfig)
		if err != nil {
			return nil, err
		}
		envoyHttpTracingProvider.ConfigType = &envoytrace.Tracing_Http_TypedConfig{
			TypedConfig: converted,
		}
	}

	return envoyHttpTracingProvider, nil
}

