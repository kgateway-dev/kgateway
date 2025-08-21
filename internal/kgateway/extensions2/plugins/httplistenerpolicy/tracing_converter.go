package httplistenerpolicy

import (
	"context"
	"fmt"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	resource_detectorsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/tracers/opentelemetry/resource_detectors/v3"
	samplersv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/tracers/opentelemetry/samplers/v3"
	metadatav3 "github.com/envoyproxy/go-control-plane/envoy/type/metadata/v3"
	tracingv3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

func convertTracingConfig(
	ctx context.Context,
	policy *v1alpha1.HTTPListenerPolicy,
	commoncol *common.CommonCollections,
	krtctx krt.HandlerContext,
	parentSrc ir.ObjectSource,
) (*envoytracev3.OpenTelemetryConfig, *envoy_hcm.HttpConnectionManager_Tracing, error) {
	config := policy.Spec.Tracing
	if config == nil {
		return nil, nil, nil
	}

	if config.Provider.OpenTelemetry.GrpcService.BackendRef == nil {
		return nil, nil, fmt.Errorf("Tracing.OpenTelemetryConfig.GrpcService.BackendRef must be specified")
	}

	backend, err := commoncol.BackendIndex.GetBackendFromRef(krtctx, parentSrc, config.Provider.OpenTelemetry.GrpcService.BackendRef.BackendObjectReference)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrUnresolvedBackendRef, err)
	}

	return translateTracing(config, backend)
}

func translateTracing(
	config *v1alpha1.Tracing,
	backend *ir.BackendObjectIR,
) (*envoytracev3.OpenTelemetryConfig, *envoy_hcm.HttpConnectionManager_Tracing, error) {
	if config == nil {
		return nil, nil, nil
	}

	if config.Provider.OpenTelemetry == nil || config.Provider.OpenTelemetry.GrpcService.BackendRef == nil {
		return nil, nil, fmt.Errorf("Tracing.OpenTelemetryConfig.GrpcService.BackendRef must be specified")
	}

	provider, err := convertOTelTracingConfig(config.Provider.OpenTelemetry, backend)
	if err != nil {
		return nil, nil, err
	}

	tracingConfig := &envoy_hcm.HttpConnectionManager_Tracing{}
	if config.ClientSampling != nil {
		tracingConfig.ClientSampling = &typev3.Percent{
			Value: float64(*config.ClientSampling),
		}
	}
	if config.RandomSampling != nil {
		tracingConfig.RandomSampling = &typev3.Percent{
			Value: float64(*config.RandomSampling),
		}
	}
	if config.OverallSampling != nil {
		tracingConfig.OverallSampling = &typev3.Percent{
			Value: float64(*config.OverallSampling),
		}
	}
	if config.Verbose != nil {
		tracingConfig.Verbose = *config.Verbose
	}
	if config.MaxPathTagLength != nil {
		tracingConfig.MaxPathTagLength = &wrapperspb.UInt32Value{
			Value: *config.MaxPathTagLength,
		}
	}
	if len(config.Attributes) != 0 {
		tracingConfig.CustomTags = make([]*tracingv3.CustomTag, len(config.Attributes))
		for i, ct := range config.Attributes {
			if ct.Literal != nil {
				tracingConfig.GetCustomTags()[i] = &tracingv3.CustomTag{
					Tag: ct.Name,
					Type: &tracingv3.CustomTag_Literal_{
						Literal: &tracingv3.CustomTag_Literal{
							Value: ct.Literal.Value,
						},
					},
				}
				continue
			}

			if ct.Environment != nil {
				tagType := &tracingv3.CustomTag_Environment_{
					Environment: &tracingv3.CustomTag_Environment{
						Name: ct.Environment.Name,
					},
				}
				if ct.Environment.DefaultValue != nil {
					tagType.Environment.DefaultValue = *ct.Environment.DefaultValue
				}

				tracingConfig.GetCustomTags()[i] = &tracingv3.CustomTag{
					Tag:  ct.Name,
					Type: tagType,
				}
				continue
			}

			if ct.RequestHeader != nil {
				tagType := &tracingv3.CustomTag_RequestHeader{
					RequestHeader: &tracingv3.CustomTag_Header{
						Name: ct.RequestHeader.Name,
					},
				}
				if ct.RequestHeader.DefaultValue != nil {
					tagType.RequestHeader.DefaultValue = *ct.RequestHeader.DefaultValue
				}

				tracingConfig.GetCustomTags()[i] = &tracingv3.CustomTag{
					Tag:  ct.Name,
					Type: tagType,
				}
				continue
			}

			if ct.Metadata != nil {
				tagType := &tracingv3.CustomTag_Metadata_{
					Metadata: &tracingv3.CustomTag_Metadata{
						MetadataKey: &metadatav3.MetadataKey{
							Key: ct.Metadata.MetadataKey.Key,
						},
					},
				}

				if len(ct.Metadata.MetadataKey.Path) != 0 {
					paths := make([]*metadatav3.MetadataKey_PathSegment, len(ct.Metadata.MetadataKey.Path))
					for i, p := range ct.Metadata.MetadataKey.Path {
						paths[i] = &metadatav3.MetadataKey_PathSegment{
							Segment: &metadatav3.MetadataKey_PathSegment_Key{
								Key: p.Key,
							},
						}
					}
					tagType.Metadata.GetMetadataKey().Path = paths
				}

				switch ct.Metadata.Kind {
				case v1alpha1.MetadataKindRequest:
					tagType.Metadata.Kind = &metadatav3.MetadataKind{
						Kind: &metadatav3.MetadataKind_Request_{
							Request: &metadatav3.MetadataKind_Request{},
						},
					}
				case v1alpha1.MetadataKindRoute:
					tagType.Metadata.Kind = &metadatav3.MetadataKind{
						Kind: &metadatav3.MetadataKind_Route_{
							Route: &metadatav3.MetadataKind_Route{},
						},
					}
				case v1alpha1.MetadataKindCluster:
					tagType.Metadata.Kind = &metadatav3.MetadataKind{
						Kind: &metadatav3.MetadataKind_Cluster_{
							Cluster: &metadatav3.MetadataKind_Cluster{},
						},
					}
				case v1alpha1.MetadataKindHost:
					tagType.Metadata.Kind = &metadatav3.MetadataKind{
						Kind: &metadatav3.MetadataKind_Host_{
							Host: &metadatav3.MetadataKind_Host{},
						},
					}
				}

				if ct.Metadata.DefaultValue != nil {
					tagType.Metadata.DefaultValue = *ct.Metadata.DefaultValue
				}

				tracingConfig.GetCustomTags()[i] = &tracingv3.CustomTag{
					Tag:  ct.Name,
					Type: tagType,
				}
				continue
			}
		}
	}
	if config.SpawnUpstreamSpan != nil {
		tracingConfig.SpawnUpstreamSpan = &wrapperspb.BoolValue{
			Value: *config.SpawnUpstreamSpan,
		}
	}

	return provider, tracingConfig, nil
}

func convertOTelTracingConfig(
	config *v1alpha1.OpenTelemetryTracingConfig,
	backend *ir.BackendObjectIR,
) (*envoytracev3.OpenTelemetryConfig, error) {
	if config == nil {
		return nil, nil
	}

	envoyGrpcService, err := ToEnvoyGrpc(config.GrpcService, backend)
	if err != nil {
		return nil, err
	}

	tracingCfg := &envoytracev3.OpenTelemetryConfig{
		GrpcService: envoyGrpcService,
	}
	if config.ServiceName != nil {
		tracingCfg.ServiceName = *config.ServiceName
	}
	if len(config.ResourceDetectors) != 0 {
		translatedResourceDetectors := make([]*envoycorev3.TypedExtensionConfig, len(config.ResourceDetectors))
		for i, rd := range config.ResourceDetectors {
			if rd.EnvironmentResourceDetector != nil {
				detector, _ := utils.MessageToAny(&resource_detectorsv3.EnvironmentResourceDetectorConfig{})
				translatedResourceDetectors[i] = &envoycorev3.TypedExtensionConfig{
					Name:        "envoy.tracers.opentelemetry.resource_detectors.environment",
					TypedConfig: detector,
				}
			}
		}
		tracingCfg.ResourceDetectors = translatedResourceDetectors
	}

	if config.Sampler != nil {
		if config.Sampler.AlwaysOn != nil {
			alwaysOnSampler, _ := utils.MessageToAny(&samplersv3.AlwaysOnSamplerConfig{})
			tracingCfg.Sampler = &envoycorev3.TypedExtensionConfig{
				Name:        "envoy.tracers.opentelemetry.samplers.always_on",
				TypedConfig: alwaysOnSampler,
			}
		}
	}

	return tracingCfg, nil
}

func updateTracingConfig(pCtx *ir.HcmContext, tracingProvider *envoytracev3.OpenTelemetryConfig, tracingConfig *envoy_hcm.HttpConnectionManager_Tracing) {
	if tracingProvider == nil || tracingConfig == nil {
		return
	}
	if tracingProvider.ServiceName == "" {
		tracingProvider.ServiceName = GenerateDefaultServiceName(pCtx.Gateway.SourceObject.GetName(), pCtx.Gateway.SourceObject.GetNamespace())
	}
	otelCfg := utils.MustMessageToAny(tracingProvider)

	tracingConfig.Provider = &envoytracev3.Tracing_Http{
		Name: "envoy.tracers.opentelemetry",
		ConfigType: &envoytracev3.Tracing_Http_TypedConfig{
			TypedConfig: otelCfg,
		},
	}
}

// GenerateDefaultServiceName returns the default service name that matches the cluster name
// specified in the envoy bootstrap config
// Ie: `<gateway-name>.<gateway-namespace>`
func GenerateDefaultServiceName(name, namespace string) string {
	return fmt.Sprintf("%s.%s", name, namespace)
}
