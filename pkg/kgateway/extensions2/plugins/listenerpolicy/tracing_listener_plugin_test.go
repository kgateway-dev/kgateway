package listenerpolicy

import (
	"context"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	resource_detectorsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/tracers/opentelemetry/resource_detectors/v3"
	samplersv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/tracers/opentelemetry/samplers/v3"
	metadatav3 "github.com/envoyproxy/go-control-plane/envoy/type/metadata/v3"
	tracingv3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestTracingConverter(t *testing.T) {
	t.Run("Tracing Conversion", func(t *testing.T) {
		testCases := []struct {
			name     string
			config   *kgateway.Tracing
			expected *envoy_hcm.HttpConnectionManager_Tracing
		}{
			{
				name:     "NilConfig",
				config:   nil,
				expected: nil,
			},
			{
				name: "OTel Tracing minimal config",
				config: &kgateway.Tracing{
					Provider: kgateway.TracingProvider{
						OpenTelemetry: &kgateway.OpenTelemetryTracingConfig{
							GrpcService: kgateway.CommonGrpcService{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "test-service",
									},
								},
							},
						},
					},
				},
				expected: &envoy_hcm.HttpConnectionManager_Tracing{
					Provider: &envoytracev3.Tracing_Http{
						Name: "envoy.tracers.opentelemetry",
						ConfigType: &envoytracev3.Tracing_Http_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoytracev3.OpenTelemetryConfig{
								GrpcService: &envoycorev3.GrpcService{
									TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
											ClusterName: "backend_default_test-service_0",
										},
									},
								},
								ServiceName: "gw.default",
							}),
						},
					},
				},
			},
			{
				name: "OTel Tracing with nil attributes",
				config: &kgateway.Tracing{
					Provider: kgateway.TracingProvider{
						OpenTelemetry: &kgateway.OpenTelemetryTracingConfig{
							GrpcService: kgateway.CommonGrpcService{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "test-service",
									},
								},
							},
						},
					},
					Attributes: nil,
				},
				expected: &envoy_hcm.HttpConnectionManager_Tracing{
					Provider: &envoytracev3.Tracing_Http{
						Name: "envoy.tracers.opentelemetry",
						ConfigType: &envoytracev3.Tracing_Http_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoytracev3.OpenTelemetryConfig{
								GrpcService: &envoycorev3.GrpcService{
									TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
											ClusterName: "backend_default_test-service_0",
										},
									},
								},
								ServiceName: "gw.default",
							}),
						},
					},
				},
			},
			{
				name: "OTel Tracing with nil attributes",
				config: &kgateway.Tracing{
					Provider: kgateway.TracingProvider{
						OpenTelemetry: &kgateway.OpenTelemetryTracingConfig{
							GrpcService: kgateway.CommonGrpcService{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "test-service",
									},
								},
							},
						},
					},
					Attributes: []kgateway.CustomAttribute{},
				},
				expected: &envoy_hcm.HttpConnectionManager_Tracing{
					Provider: &envoytracev3.Tracing_Http{
						Name: "envoy.tracers.opentelemetry",
						ConfigType: &envoytracev3.Tracing_Http_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoytracev3.OpenTelemetryConfig{
								GrpcService: &envoycorev3.GrpcService{
									TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
											ClusterName: "backend_default_test-service_0",
										},
									},
								},
								ServiceName: "gw.default",
							}),
						},
					},
					CustomTags: []*tracingv3.CustomTag{},
				},
			},
			{
				name: "OTel Tracing full config",
				config: &kgateway.Tracing{
					Provider: kgateway.TracingProvider{
						OpenTelemetry: &kgateway.OpenTelemetryTracingConfig{
							GrpcService: kgateway.CommonGrpcService{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "test-service",
									},
								},
							},
							ServiceName: new("my:service"),
							ResourceDetectors: []kgateway.ResourceDetector{{
								EnvironmentResourceDetector: &kgateway.EnvironmentResourceDetectorConfig{},
							}},
							Sampler: &kgateway.Sampler{
								AlwaysOn: &kgateway.AlwaysOnConfig{},
							},
						},
					},
					ClientSampling:   new(int32(45)),
					RandomSampling:   new(int32(55)),
					OverallSampling:  new(int32(65)),
					Verbose:          new(true),
					MaxPathTagLength: new(int32(127)),
					Attributes: []kgateway.CustomAttribute{
						{
							Name: "Literal",
							Literal: &kgateway.CustomAttributeLiteral{
								Value: "Literal Value",
							},
						},
						{
							Name: "Environment",
							Environment: &kgateway.CustomAttributeEnvironment{
								Name:         "Env",
								DefaultValue: new("Environment Value"),
							},
						},
						{
							Name: "Request Header",
							RequestHeader: &kgateway.CustomAttributeHeader{
								Name:         "Header",
								DefaultValue: new("Request"),
							},
						},
						{
							Name: "Metadata Request",
							Metadata: &kgateway.CustomAttributeMetadata{
								Kind: kgateway.MetadataKindRequest,
								MetadataKey: kgateway.MetadataKey{
									Key: "Request",
									Path: []kgateway.MetadataPathSegment{{
										Key: "Request-key",
									}},
								},
							},
						},
						{
							Name: "Metadata Route",
							Metadata: &kgateway.CustomAttributeMetadata{
								Kind: kgateway.MetadataKindRoute,
								MetadataKey: kgateway.MetadataKey{
									Key: "Route",
									Path: []kgateway.MetadataPathSegment{{
										Key: "Route-key",
									}},
								},
							},
						},
						{
							Name: "Metadata Cluster",
							Metadata: &kgateway.CustomAttributeMetadata{
								Kind: kgateway.MetadataKindCluster,
								MetadataKey: kgateway.MetadataKey{
									Key: "Cluster",
									Path: []kgateway.MetadataPathSegment{{
										Key: "Cluster-key",
									}},
								},
							},
						},
						{
							Name: "Metadata Host",
							Metadata: &kgateway.CustomAttributeMetadata{
								Kind: kgateway.MetadataKindHost,
								MetadataKey: kgateway.MetadataKey{
									Key: "Host",
									Path: []kgateway.MetadataPathSegment{{
										Key: "Host-key-1",
									}, {
										Key: "Host-key-2",
									}},
								},
							},
						},
					},
					SpawnUpstreamSpan: new(true),
				},
				expected: &envoy_hcm.HttpConnectionManager_Tracing{
					Provider: &envoytracev3.Tracing_Http{
						Name: "envoy.tracers.opentelemetry",
						ConfigType: &envoytracev3.Tracing_Http_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoytracev3.OpenTelemetryConfig{
								GrpcService: &envoycorev3.GrpcService{
									TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
											ClusterName: "backend_default_test-service_0",
										},
									},
								},
								ServiceName: "my:service",
								ResourceDetectors: []*envoycorev3.TypedExtensionConfig{{
									Name:        "envoy.tracers.opentelemetry.resource_detectors.environment",
									TypedConfig: mustMessageToAny(t, &resource_detectorsv3.EnvironmentResourceDetectorConfig{}),
								}},
								Sampler: &envoycorev3.TypedExtensionConfig{
									Name:        "envoy.tracers.opentelemetry.samplers.always_on",
									TypedConfig: mustMessageToAny(t, &samplersv3.AlwaysOnSamplerConfig{}),
								},
							}),
						},
					},
					ClientSampling:   &typev3.Percent{Value: 45},
					RandomSampling:   &typev3.Percent{Value: 55},
					OverallSampling:  &typev3.Percent{Value: 65},
					Verbose:          true,
					MaxPathTagLength: &wrapperspb.UInt32Value{Value: 127},
					CustomTags: []*tracingv3.CustomTag{
						{
							Tag: "Literal",
							Type: &tracingv3.CustomTag_Literal_{
								Literal: &tracingv3.CustomTag_Literal{
									Value: "Literal Value",
								},
							},
						},
						{
							Tag: "Environment",
							Type: &tracingv3.CustomTag_Environment_{
								Environment: &tracingv3.CustomTag_Environment{
									Name:         "Env",
									DefaultValue: "Environment Value",
								},
							},
						},
						{
							Tag: "Request Header",
							Type: &tracingv3.CustomTag_RequestHeader{
								RequestHeader: &tracingv3.CustomTag_Header{
									Name:         "Header",
									DefaultValue: "Request",
								},
							},
						},
						{
							Tag: "Metadata Request",
							Type: &tracingv3.CustomTag_Metadata_{
								Metadata: &tracingv3.CustomTag_Metadata{
									Kind: &metadatav3.MetadataKind{
										Kind: &metadatav3.MetadataKind_Request_{
											Request: &metadatav3.MetadataKind_Request{},
										},
									},
									MetadataKey: &metadatav3.MetadataKey{
										Key: "Request",
										Path: []*metadatav3.MetadataKey_PathSegment{{
											Segment: &metadatav3.MetadataKey_PathSegment_Key{
												Key: "Request-key",
											},
										}},
									},
								},
							},
						},
						{
							Tag: "Metadata Route",
							Type: &tracingv3.CustomTag_Metadata_{
								Metadata: &tracingv3.CustomTag_Metadata{
									Kind: &metadatav3.MetadataKind{
										Kind: &metadatav3.MetadataKind_Route_{
											Route: &metadatav3.MetadataKind_Route{},
										},
									},
									MetadataKey: &metadatav3.MetadataKey{
										Key: "Route",
										Path: []*metadatav3.MetadataKey_PathSegment{{
											Segment: &metadatav3.MetadataKey_PathSegment_Key{
												Key: "Route-key",
											},
										}},
									},
								},
							},
						},
						{
							Tag: "Metadata Cluster",
							Type: &tracingv3.CustomTag_Metadata_{
								Metadata: &tracingv3.CustomTag_Metadata{
									Kind: &metadatav3.MetadataKind{
										Kind: &metadatav3.MetadataKind_Cluster_{
											Cluster: &metadatav3.MetadataKind_Cluster{},
										},
									},
									MetadataKey: &metadatav3.MetadataKey{
										Key: "Cluster",
										Path: []*metadatav3.MetadataKey_PathSegment{{
											Segment: &metadatav3.MetadataKey_PathSegment_Key{
												Key: "Cluster-key",
											},
										}},
									},
								},
							},
						},
						{
							Tag: "Metadata Host",
							Type: &tracingv3.CustomTag_Metadata_{
								Metadata: &tracingv3.CustomTag_Metadata{
									Kind: &metadatav3.MetadataKind{
										Kind: &metadatav3.MetadataKind_Host_{
											Host: &metadatav3.MetadataKind_Host{},
										},
									},
									MetadataKey: &metadatav3.MetadataKey{
										Key: "Host",
										Path: []*metadatav3.MetadataKey_PathSegment{
											{
												Segment: &metadatav3.MetadataKey_PathSegment_Key{
													Key: "Host-key-1",
												},
											}, {
												Segment: &metadatav3.MetadataKey_PathSegment_Key{
													Key: "Host-key-2",
												},
											},
										},
									},
								},
							},
						},
					},
					SpawnUpstreamSpan: &wrapperspb.BoolValue{Value: true},
				},
			},
		}
		for _, tc := range testCases {
			_, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			t.Run(tc.name, func(t *testing.T) {
				provider, config, err := translateTracing(
					tc.config,
					&ir.BackendObjectIR{
						ObjectSource: ir.ObjectSource{
							Kind:      "Backend",
							Name:      "test-service",
							Namespace: "default",
						},
					},
				)
				updateTracingConfig(&ir.HcmContext{
					Gateway: ir.GatewayIR{
						SourceObject: &ir.Gateway{
							ObjectSource: ir.ObjectSource{
								Namespace: "default",
								Name:      "gw",
							},
						},
					},
				}, provider, config)
				require.NoError(t, err, "failed to convert access log config")
				if tc.expected != nil {
					assert.True(t, proto.Equal(tc.expected, config),
						"Tracing config mismatch\n %v\n %v\n", tc.expected, config)
				}
			})
		}
	})
}
