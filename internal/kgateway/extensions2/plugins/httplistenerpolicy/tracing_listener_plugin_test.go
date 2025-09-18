package httplistenerpolicy

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
	"k8s.io/utils/pointer"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	pluginsdkir "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestTracingConverter(t *testing.T) {
	t.Run("Tracing Conversion", func(t *testing.T) {
		testCases := []struct {
			name     string
			config   *v1alpha1.Tracing
			expected *envoy_hcm.HttpConnectionManager_Tracing
		}{
			{
				name:     "NilConfig",
				config:   nil,
				expected: nil,
			},
			{
				name: "OTel Tracing minimal config",
				config: &v1alpha1.Tracing{
					Provider: v1alpha1.TracingProvider{
						OpenTelemetry: &v1alpha1.OpenTelemetryTracingConfig{
							GrpcService: v1alpha1.CommonGrpcService{
								BackendRef: &gwv1.BackendRef{
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
				config: &v1alpha1.Tracing{
					Provider: v1alpha1.TracingProvider{
						OpenTelemetry: &v1alpha1.OpenTelemetryTracingConfig{
							GrpcService: v1alpha1.CommonGrpcService{
								BackendRef: &gwv1.BackendRef{
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
				config: &v1alpha1.Tracing{
					Provider: v1alpha1.TracingProvider{
						OpenTelemetry: &v1alpha1.OpenTelemetryTracingConfig{
							GrpcService: v1alpha1.CommonGrpcService{
								BackendRef: &gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "test-service",
									},
								},
							},
						},
					},
					Attributes: []v1alpha1.CustomAttribute{},
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
				config: &v1alpha1.Tracing{
					Provider: v1alpha1.TracingProvider{
						OpenTelemetry: &v1alpha1.OpenTelemetryTracingConfig{
							GrpcService: v1alpha1.CommonGrpcService{
								BackendRef: &gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "test-service",
									},
								},
							},
							ServiceName: pointer.String("my:service"),
							ResourceDetectors: []v1alpha1.ResourceDetector{{
								EnvironmentResourceDetector: &v1alpha1.EnvironmentResourceDetectorConfig{},
							}},
							Sampler: &v1alpha1.Sampler{
								AlwaysOn: &v1alpha1.AlwaysOnConfig{},
							},
						},
					},
					ClientSampling:   pointer.Uint32(45),
					RandomSampling:   pointer.Uint32(55),
					OverallSampling:  pointer.Uint32(65),
					Verbose:          pointer.Bool(true),
					MaxPathTagLength: pointer.Uint32(127),
					Attributes: []v1alpha1.CustomAttribute{
						{
							Name: "Literal",
							Literal: &v1alpha1.CustomAttributeLiteral{
								Value: "Literal Value",
							},
						},
						{
							Name: "Environment",
							Environment: &v1alpha1.CustomAttributeEnvironment{
								Name:         "Env",
								DefaultValue: pointer.String("Environment Value"),
							},
						},
						{
							Name: "Request Header",
							RequestHeader: &v1alpha1.CustomAttributeHeader{
								Name:         "Header",
								DefaultValue: pointer.String("Request"),
							},
						},
						{
							Name: "Metadata Request",
							Metadata: &v1alpha1.CustomAttributeMetadata{
								Kind: v1alpha1.MetadataKindRequest,
								MetadataKey: v1alpha1.MetadataKey{
									Key: "Request",
									Path: []v1alpha1.MetadataPathSegment{{
										Key: "Request-key",
									}},
								},
							},
						},
						{
							Name: "Metadata Route",
							Metadata: &v1alpha1.CustomAttributeMetadata{
								Kind: v1alpha1.MetadataKindRoute,
								MetadataKey: v1alpha1.MetadataKey{
									Key: "Route",
									Path: []v1alpha1.MetadataPathSegment{{
										Key: "Route-key",
									}},
								},
							},
						},
						{
							Name: "Metadata Cluster",
							Metadata: &v1alpha1.CustomAttributeMetadata{
								Kind: v1alpha1.MetadataKindCluster,
								MetadataKey: v1alpha1.MetadataKey{
									Key: "Cluster",
									Path: []v1alpha1.MetadataPathSegment{{
										Key: "Cluster-key",
									}},
								},
							},
						},
						{
							Name: "Metadata Host",
							Metadata: &v1alpha1.CustomAttributeMetadata{
								Kind: v1alpha1.MetadataKindHost,
								MetadataKey: v1alpha1.MetadataKey{
									Key: "Host",
									Path: []v1alpha1.MetadataPathSegment{{
										Key: "Host-key-1",
									}, {
										Key: "Host-key-2",
									}},
								},
							},
						},
					},
					SpawnUpstreamSpan: pointer.Bool(true),
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
										Path: []*metadatav3.MetadataKey_PathSegment{{
											Segment: &metadatav3.MetadataKey_PathSegment_Key{
												Key: "Host-key-1",
											}}, {
											Segment: &metadatav3.MetadataKey_PathSegment_Key{
												Key: "Host-key-2",
											}},
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
					Gateway: pluginsdkir.GatewayIR{
						SourceObject: &pluginsdkir.Gateway{
							ObjectSource: pluginsdkir.ObjectSource{
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
