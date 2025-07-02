package httplistenerpolicy

import (
	"context"
	"testing"
	"time"

	v33 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	cel "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/filters/cel/v3"
	envoygrpc "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	envoy_open_telemetry "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/open_telemetry/v3"
	envoy_metadata_formatter "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/metadata/v3"
	envoy_req_without_query "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/req_without_query/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	otelv1 "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func TestConvertJsonFormat_EdgeCases(t *testing.T) {
	t.Run("Access Log Conversion", func(t *testing.T) {
		testCases := []struct {
			name     string
			config   []v1alpha1.AccessLog
			expected []*v33.AccessLog
		}{
			{
				name:     "NilConfig",
				config:   nil,
				expected: nil,
			},
			{
				name:     "EmptyAccessLog",
				config:   []v1alpha1.AccessLog{},
				expected: nil,
			},
			{
				name: "FileSinkWithJSONFormat",
				config: []v1alpha1.AccessLog{{
					FileSink: &v1alpha1.FileSink{
						Path: "/var/log/access.json",
						JsonFormat: &runtime.RawExtension{
							Raw: []byte(`{"request_method": "%REQ(:METHOD)%", "response_code": "%RESPONSE_CODE%"}`),
						},
					},
				},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.json",
								AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
									LogFormat: &envoycore.SubstitutionFormatString{
										Formatters: []*envoycore.TypedExtensionConfig{
											{
												Name:        "envoy.formatter.req_without_query",
												TypedConfig: mustMessageToAny(t, &envoy_req_without_query.ReqWithoutQuery{}),
											},
											{
												Name:        "envoy.formatter.metadata",
												TypedConfig: mustMessageToAny(t, &envoy_metadata_formatter.Metadata{}),
											},
										},
										Format: &envoycore.SubstitutionFormatString_JsonFormat{
											JsonFormat: &structpb.Struct{
												Fields: map[string]*structpb.Value{
													"request_method": {
														Kind: &structpb.Value_StringValue{
															StringValue: "%REQ(:METHOD)%",
														},
													},
													"response_code": {
														Kind: &structpb.Value_StringValue{
															StringValue: "%RESPONSE_CODE%",
														},
													},
												},
											},
										},
									},
								},
							}),
						},
					},
				},
			},
			{
				name: "GRPCAdditionalHeaders",
				config: []v1alpha1.AccessLog{
					{
						GrpcService: &v1alpha1.AccessLogGrpcService{
							CommonAccessLogGrpcService: v1alpha1.CommonAccessLogGrpcService{
								CommonGrpcService: v1alpha1.CommonGrpcService{
									BackendRef: &gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
										},
									},
								},
								LogName: "grpc-log",
							},
							AdditionalRequestHeadersToLog:   []string{"x-request-id"},
							AdditionalResponseHeadersToLog:  []string{"x-response-id"},
							AdditionalResponseTrailersToLog: []string{"x-trailer"},
						},
					},
					{
						FileSink: &v1alpha1.FileSink{
							Path:         "/var/log/file-access.log",
							StringFormat: "[%START_TIME%] %RESPONSE_CODE%",
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.http_grpc",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoygrpc.HttpGrpcAccessLogConfig{
								AdditionalRequestHeadersToLog:   []string{"x-request-id"},
								AdditionalResponseHeadersToLog:  []string{"x-response-id"},
								AdditionalResponseTrailersToLog: []string{"x-trailer"},
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									TransportApiVersion: envoycore.ApiVersion_V3,
									LogName:             "grpc-log",
									GrpcService: &envoycore.GrpcService{
										TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
												ClusterName: "backend_default_test-service_0",
											},
										},
									},
								},
							}),
						},
					},
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/file-access.log",
								AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
									LogFormat: &envoycore.SubstitutionFormatString{
										Formatters: []*envoycore.TypedExtensionConfig{
											{
												Name:        "envoy.formatter.req_without_query",
												TypedConfig: mustMessageToAny(t, &envoy_req_without_query.ReqWithoutQuery{}),
											},
											{
												Name:        "envoy.formatter.metadata",
												TypedConfig: mustMessageToAny(t, &envoy_metadata_formatter.Metadata{}),
											},
										},
										Format: &envoycore.SubstitutionFormatString_TextFormatSource{
											TextFormatSource: &envoycore.DataSource{
												Specifier: &envoycore.DataSource_InlineString{
													InlineString: "[%START_TIME%] %RESPONSE_CODE%",
												},
											},
										},
									},
								},
							}),
						},
					},
				},
			},
			{
				name: "FileSinkWithStringFormat",
				config: []v1alpha1.AccessLog{
					{
						FileSink: &v1alpha1.FileSink{
							Path:         "/var/log/access.log",
							StringFormat: "test log format",
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
								AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
									LogFormat: &envoycore.SubstitutionFormatString{
										Formatters: []*envoycore.TypedExtensionConfig{
											{
												Name:        "envoy.formatter.req_without_query",
												TypedConfig: mustMessageToAny(t, &envoy_req_without_query.ReqWithoutQuery{}),
											},
											{
												Name:        "envoy.formatter.metadata",
												TypedConfig: mustMessageToAny(t, &envoy_metadata_formatter.Metadata{}),
											},
										},
										Format: &envoycore.SubstitutionFormatString_TextFormatSource{
											TextFormatSource: &envoycore.DataSource{
												Specifier: &envoycore.DataSource_InlineString{
													InlineString: "test log format",
												},
											},
										},
									},
								},
							}),
						},
					},
				},
			},
			{
				name: "FileSinkWithJSONFormat",
				config: []v1alpha1.AccessLog{
					{
						FileSink: &v1alpha1.FileSink{
							Path: "/var/log/access.log",
							JsonFormat: &runtime.RawExtension{
								Raw: []byte(`{"request_method": "%REQ(:METHOD)%", "response_code": "%RESPONSE_CODE%"}`),
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
								AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
									LogFormat: &envoycore.SubstitutionFormatString{
										Formatters: []*envoycore.TypedExtensionConfig{
											{
												Name:        "envoy.formatter.req_without_query",
												TypedConfig: mustMessageToAny(t, &envoy_req_without_query.ReqWithoutQuery{}),
											},
											{
												Name:        "envoy.formatter.metadata",
												TypedConfig: mustMessageToAny(t, &envoy_metadata_formatter.Metadata{}),
											},
										},
										Format: &envoycore.SubstitutionFormatString_JsonFormat{
											JsonFormat: &structpb.Struct{
												Fields: map[string]*structpb.Value{
													"request_method": {
														Kind: &structpb.Value_StringValue{
															StringValue: "%REQ(:METHOD)%",
														},
													},
													"response_code": {
														Kind: &structpb.Value_StringValue{
															StringValue: "%RESPONSE_CODE%",
														},
													},
												},
											},
										},
									},
								},
							}),
						},
					},
				},
			},
			{
				name: "GrpcServiceConfig",
				config: []v1alpha1.AccessLog{
					{
						GrpcService: &v1alpha1.AccessLogGrpcService{
							CommonAccessLogGrpcService: v1alpha1.CommonAccessLogGrpcService{
								CommonGrpcService: v1alpha1.CommonGrpcService{
									BackendRef: &gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
										},
									},
								},
								LogName: "grpc-log",
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.http_grpc",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoygrpc.HttpGrpcAccessLogConfig{
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									LogName: "grpc-log",
									GrpcService: &envoycore.GrpcService{
										TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
												ClusterName: "backend_default_test-service_0",
											},
										},
									},
									TransportApiVersion: envoycore.ApiVersion_V3,
								},
							}),
						},
					},
				},
			},
			{
				name: "GrpcServiceConfig with invalid retry policy",
				config: []v1alpha1.AccessLog{
					{
						GrpcService: &v1alpha1.AccessLogGrpcService{
							CommonAccessLogGrpcService: v1alpha1.CommonAccessLogGrpcService{
								CommonGrpcService: v1alpha1.CommonGrpcService{
									BackendRef: &gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
										},
									},
									RetryPolicy: &v1alpha1.RetryPolicy{
										RetryBackOff: &v1alpha1.BackoffStrategy{
											BaseInterval: metav1.Duration{Duration: 5 * time.Second},
											MaxInterval:  &metav1.Duration{Duration: 1 * time.Second},
										},
									},
								},
								LogName: "grpc-log",
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.http_grpc",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoygrpc.HttpGrpcAccessLogConfig{
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									LogName: "grpc-log",
									GrpcService: &envoycore.GrpcService{
										TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
												ClusterName: "backend_default_test-service_0",
											},
										},
										RetryPolicy: &envoycore.RetryPolicy{
											RetryBackOff: &envoycore.BackoffStrategy{
												BaseInterval: &durationpb.Duration{Seconds: 5},
											},
										},
									},
									TransportApiVersion: envoycore.ApiVersion_V3,
								},
							}),
						},
					},
				},
			},
			{
				name: "GrpcServiceConfig with all the common options",
				config: []v1alpha1.AccessLog{
					{
						GrpcService: &v1alpha1.AccessLogGrpcService{
							CommonAccessLogGrpcService: v1alpha1.CommonAccessLogGrpcService{
								CommonGrpcService: v1alpha1.CommonGrpcService{
									BackendRef: &gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
										},
									},
									Authority:               pointer.String("www.example.com"),
									MaxReceiveMessageLength: pointer.Uint32(127),
									SkipEnvoyHeaders:        pointer.Bool(true),
									Timeout:                 &metav1.Duration{Duration: 10 * time.Second},
									InitialMetadata: []v1alpha1.HeaderValue{{
										Key:   "key",
										Value: "value",
									}},
									RetryPolicy: &v1alpha1.RetryPolicy{
										RetryBackOff: &v1alpha1.BackoffStrategy{
											BaseInterval: metav1.Duration{Duration: 5 * time.Second},
											MaxInterval:  &metav1.Duration{Duration: 10 * time.Second},
										},
										NumRetries: pointer.Uint32(3),
									},
								},
								LogName: "grpc-log",
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.http_grpc",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoygrpc.HttpGrpcAccessLogConfig{
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									LogName: "grpc-log",
									GrpcService: &envoycore.GrpcService{
										TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
												ClusterName:             "backend_default_test-service_0",
												Authority:               "www.example.com",
												MaxReceiveMessageLength: &wrapperspb.UInt32Value{Value: 127},
												SkipEnvoyHeaders:        true,
											},
										},
										Timeout: &durationpb.Duration{Seconds: 10},
										InitialMetadata: []*envoycore.HeaderValue{{
											Key:   "key",
											Value: "value",
										}},
										RetryPolicy: &envoycore.RetryPolicy{
											RetryBackOff: &envoycore.BackoffStrategy{
												BaseInterval: &durationpb.Duration{Seconds: 5},
												MaxInterval:  &durationpb.Duration{Seconds: 10},
											},
											NumRetries: &wrapperspb.UInt32Value{Value: 3},
										},
									},
									TransportApiVersion: envoycore.ApiVersion_V3,
								},
							}),
						},
					},
				},
			},
			{
				name: "AccessLogWithStatusCodeFilter",
				config: []v1alpha1.AccessLog{
					{
						FileSink: &v1alpha1.FileSink{
							Path:         "/var/log/access.log",
							StringFormat: "hello kgateway",
						},
						Filter: &v1alpha1.AccessLogFilter{
							FilterType: &v1alpha1.FilterType{
								StatusCodeFilter: &v1alpha1.StatusCodeFilter{
									Op:    v1alpha1.EQ,
									Value: 5,
								},
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
								AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
									LogFormat: &envoycore.SubstitutionFormatString{
										Formatters: []*envoycore.TypedExtensionConfig{
											{
												Name:        "envoy.formatter.req_without_query",
												TypedConfig: mustMessageToAny(t, &envoy_req_without_query.ReqWithoutQuery{}),
											},
											{
												Name:        "envoy.formatter.metadata",
												TypedConfig: mustMessageToAny(t, &envoy_metadata_formatter.Metadata{}),
											},
										},
										Format: &envoycore.SubstitutionFormatString_TextFormatSource{
											TextFormatSource: &envoycore.DataSource{
												Specifier: &envoycore.DataSource_InlineString{
													InlineString: "hello kgateway",
												},
											},
										},
									},
								},
							}),
						},
						Filter: &v33.AccessLogFilter{
							FilterSpecifier: &v33.AccessLogFilter_StatusCodeFilter{
								StatusCodeFilter: &v33.StatusCodeFilter{
									Comparison: &v33.ComparisonFilter{
										Op: v33.ComparisonFilter_EQ,
										Value: &envoycore.RuntimeUInt32{
											DefaultValue: 5,
										},
									},
								},
							},
						},
					},
				},
			},
			{
				name: "AccessLogHeaderFilter",
				config: []v1alpha1.AccessLog{
					{
						FileSink: &v1alpha1.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &v1alpha1.AccessLogFilter{
							FilterType: &v1alpha1.FilterType{
								HeaderFilter: &v1alpha1.HeaderFilter{
									Header: gwv1.HTTPHeaderMatch{
										Name:  "x-test-header",
										Type:  ptr.To(gwv1.HeaderMatchExact),
										Value: "test-value",
									},
								},
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &v33.AccessLogFilter{
							FilterSpecifier: &v33.AccessLogFilter_HeaderFilter{
								HeaderFilter: &v33.HeaderFilter{
									Header: &envoyroute.HeaderMatcher{
										Name: "x-test-header",
										HeaderMatchSpecifier: &envoyroute.HeaderMatcher_StringMatch{
											StringMatch: &envoymatcher.StringMatcher{
												MatchPattern: &envoymatcher.StringMatcher_Exact{
													Exact: "test-value",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				name: "DurationFilter",
				config: []v1alpha1.AccessLog{
					{
						FileSink: &v1alpha1.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &v1alpha1.AccessLogFilter{
							FilterType: &v1alpha1.FilterType{
								DurationFilter: &v1alpha1.DurationFilter{
									Op:    v1alpha1.EQ,
									Value: 5,
								},
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &v33.AccessLogFilter{
							FilterSpecifier: &v33.AccessLogFilter_DurationFilter{
								DurationFilter: &v33.DurationFilter{
									Comparison: &v33.ComparisonFilter{
										Op: v33.ComparisonFilter_EQ,
										Value: &envoycore.RuntimeUInt32{
											DefaultValue: 5,
										},
									},
								},
							},
						},
					},
				},
			},
			{
				name: "NotHealthCheckFilter",
				config: []v1alpha1.AccessLog{
					{
						FileSink: &v1alpha1.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &v1alpha1.AccessLogFilter{
							FilterType: &v1alpha1.FilterType{
								NotHealthCheckFilter: true,
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &v33.AccessLogFilter{
							FilterSpecifier: &v33.AccessLogFilter_NotHealthCheckFilter{
								NotHealthCheckFilter: &v33.NotHealthCheckFilter{},
							},
						},
					},
				},
			},
			{
				name: "TraceableFilter",
				config: []v1alpha1.AccessLog{
					{
						FileSink: &v1alpha1.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &v1alpha1.AccessLogFilter{
							FilterType: &v1alpha1.FilterType{
								TraceableFilter: true,
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &v33.AccessLogFilter{
							FilterSpecifier: &v33.AccessLogFilter_TraceableFilter{
								TraceableFilter: &v33.TraceableFilter{},
							},
						},
					},
				},
			},
			{
				name: "ResponseFlagFilter",
				config: []v1alpha1.AccessLog{
					{
						FileSink: &v1alpha1.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &v1alpha1.AccessLogFilter{
							FilterType: &v1alpha1.FilterType{
								ResponseFlagFilter: &v1alpha1.ResponseFlagFilter{
									Flags: []string{
										"test-flag",
									},
								},
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &v33.AccessLogFilter{
							FilterSpecifier: &v33.AccessLogFilter_ResponseFlagFilter{
								ResponseFlagFilter: &v33.ResponseFlagFilter{
									Flags: []string{
										"test-flag",
									},
								},
							},
						},
					},
				},
			},
			{
				name: "GrpcStatusFilter",
				config: []v1alpha1.AccessLog{
					{
						FileSink: &v1alpha1.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &v1alpha1.AccessLogFilter{
							FilterType: &v1alpha1.FilterType{
								GrpcStatusFilter: &v1alpha1.GrpcStatusFilter{
									Statuses: []v1alpha1.GrpcStatus{v1alpha1.NOT_FOUND},
								},
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &v33.AccessLogFilter{
							FilterSpecifier: &v33.AccessLogFilter_GrpcStatusFilter{
								GrpcStatusFilter: &v33.GrpcStatusFilter{
									Statuses: []v33.GrpcStatusFilter_Status{v33.GrpcStatusFilter_NOT_FOUND},
								},
							},
						},
					},
				},
			},
			{
				name: "CELFilter",
				config: []v1alpha1.AccessLog{
					{
						FileSink: &v1alpha1.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &v1alpha1.AccessLogFilter{
							FilterType: &v1alpha1.FilterType{
								CELFilter: &v1alpha1.CELFilter{
									Match: "connection.mtls",
								},
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &v33.AccessLogFilter{
							FilterSpecifier: &v33.AccessLogFilter_ExtensionFilter{
								ExtensionFilter: &v33.ExtensionFilter{
									Name: wellknown.CELExtensionFilter,
									ConfigType: &v33.ExtensionFilter_TypedConfig{
										TypedConfig: mustMessageToAny(t, &cel.ExpressionFilter{
											Expression: "connection.mtls",
										}),
									},
								},
							},
						},
					},
				},
			},
			{
				name: "OTel Sink",
				config: []v1alpha1.AccessLog{
					{
						OpenTelemetry: &v1alpha1.OpenTelemetryAccessLogService{
							GrpcService: v1alpha1.CommonAccessLogGrpcService{
								CommonGrpcService: v1alpha1.CommonGrpcService{
									BackendRef: &gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
										},
									},
								},
								LogName: "otel-log",
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.open_telemetry",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoy_open_telemetry.OpenTelemetryAccessLogConfig{
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									LogName: "otel-log",
									GrpcService: &envoycore.GrpcService{
										TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
												ClusterName: "backend_default_test-service_0",
											},
										},
									},
									TransportApiVersion: envoycore.ApiVersion_V3,
								},
							}),
						},
					},
				},
			},
			{
				name: "OTel Sink with all the options",
				config: []v1alpha1.AccessLog{
					{
						OpenTelemetry: &v1alpha1.OpenTelemetryAccessLogService{
							GrpcService: v1alpha1.CommonAccessLogGrpcService{
								CommonGrpcService: v1alpha1.CommonGrpcService{
									BackendRef: &gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
										},
									},
								},
								LogName: "otel-log",
							},
							Body:                 pointer.String(`"%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %RESPONSE_CODE% "%REQ(:AUTHORITY)%" "%UPSTREAM_CLUSTER%"\n'`),
							DisableBuiltinLabels: pointer.Bool(true),
							Attributes: &v1alpha1.KeyAnyValueList{
								Values: []v1alpha1.KeyAnyValue{
									{
										Key: "string-key-1",
										Value: v1alpha1.AnyValue{
											StringValue: pointer.String("string-value-1"),
										},
									},
									{
										Key: "array-key",
										Value: v1alpha1.AnyValue{
											ArrayValue: []v1alpha1.AnyValue{
												{
													StringValue: pointer.String("1-string-value"),
												},
												{
													StringValue: pointer.String("2-string-value"),
												},
											},
										},
									},
									{
										Key: "kvlist-key",
										Value: v1alpha1.AnyValue{
											KvListValue: &v1alpha1.KeyAnyValueList{
												Values: []v1alpha1.KeyAnyValue{
													{
														Key: "string-key-2",
														Value: v1alpha1.AnyValue{
															StringValue: pointer.String("string-value-2"),
														},
													},
													{
														Key: "array-key",
														Value: v1alpha1.AnyValue{
															ArrayValue: []v1alpha1.AnyValue{
																{
																	StringValue: pointer.String("3-string-value"),
																},
																{
																	StringValue: pointer.String("4-string-value"),
																},
															},
														},
													},
													{
														Key: "kvlist-key",
														Value: v1alpha1.AnyValue{
															KvListValue: &v1alpha1.KeyAnyValueList{
																Values: []v1alpha1.KeyAnyValue{
																	{
																		Key: "string-key-3",
																		Value: v1alpha1.AnyValue{
																			StringValue: pointer.String("string-value-3"),
																		},
																	},
																	{
																		Key: "string-key-4",
																		Value: v1alpha1.AnyValue{
																			StringValue: pointer.String("string-value-4"),
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				expected: []*v33.AccessLog{
					{
						Name: "envoy.access_loggers.open_telemetry",
						ConfigType: &v33.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoy_open_telemetry.OpenTelemetryAccessLogConfig{
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									LogName: "otel-log",
									GrpcService: &envoycore.GrpcService{
										TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
												ClusterName: "backend_default_test-service_0",
											},
										},
									},
									TransportApiVersion: envoycore.ApiVersion_V3,
								},
								Body: &otelv1.AnyValue{
									Value: &otelv1.AnyValue_StringValue{
										StringValue: `"%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %RESPONSE_CODE% "%REQ(:AUTHORITY)%" "%UPSTREAM_CLUSTER%"\n'`,
									},
								},
								DisableBuiltinLabels: true,
								Attributes: &otelv1.KeyValueList{
									Values: []*otelv1.KeyValue{
										{
											Key: "string-key-1",
											Value: &otelv1.AnyValue{
												Value: &otelv1.AnyValue_StringValue{
													StringValue: "string-value-1",
												},
											},
										},
										{
											Key: "array-key",
											Value: &otelv1.AnyValue{
												Value: &otelv1.AnyValue_ArrayValue{
													ArrayValue: &otelv1.ArrayValue{
														Values: []*otelv1.AnyValue{
															{
																Value: &otelv1.AnyValue_StringValue{
																	StringValue: "1-string-value",
																},
															},
															{
																Value: &otelv1.AnyValue_StringValue{
																	StringValue: "2-string-value",
																},
															},
														},
													},
												},
											},
										},
										{
											Key: "kvlist-key",
											Value: &otelv1.AnyValue{
												Value: &otelv1.AnyValue_KvlistValue{
													KvlistValue: &otelv1.KeyValueList{
														Values: []*otelv1.KeyValue{
															{
																Key: "string-key-2",
																Value: &otelv1.AnyValue{
																	Value: &otelv1.AnyValue_StringValue{
																		StringValue: "string-value-2",
																	},
																},
															},
															{
																Key: "array-key",
																Value: &otelv1.AnyValue{
																	Value: &otelv1.AnyValue_ArrayValue{
																		ArrayValue: &otelv1.ArrayValue{
																			Values: []*otelv1.AnyValue{
																				{
																					Value: &otelv1.AnyValue_StringValue{
																						StringValue: "3-string-value",
																					},
																				},
																				{
																					Value: &otelv1.AnyValue_StringValue{
																						StringValue: "4-string-value",
																					},
																				},
																			},
																		},
																	},
																},
															},
															{
																Key: "kvlist-key",
																Value: &otelv1.AnyValue{
																	Value: &otelv1.AnyValue_KvlistValue{
																		KvlistValue: &otelv1.KeyValueList{
																			Values: []*otelv1.KeyValue{
																				{
																					Key: "string-key-3",
																					Value: &otelv1.AnyValue{
																						Value: &otelv1.AnyValue_StringValue{
																							StringValue: "string-value-3",
																						},
																					},
																				},
																				{
																					Key: "string-key-4",
																					Value: &otelv1.AnyValue{
																						Value: &otelv1.AnyValue_StringValue{
																							StringValue: "string-value-4",
																						},
																					},
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							}),
						},
					},
				},
			},
		}
		for _, tc := range testCases {
			_, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			t.Run(tc.name, func(t *testing.T) {
				result, err := translateAccessLogs(
					tc.config,
					// Example grpcBackends map for upstreams
					map[string]*ir.BackendObjectIR{
						"grpc-log-0": {
							ObjectSource: ir.ObjectSource{
								Kind:      "Backend",
								Name:      "test-service",
								Namespace: "default",
							},
						},
						"otel-log-0": {
							ObjectSource: ir.ObjectSource{
								Kind:      "Backend",
								Name:      "test-service",
								Namespace: "default",
							},
						},
					},
				)
				require.NoError(t, err, "failed to convert access log config")
				// Perform deep equality check
				assert.Equal(t, len(tc.expected), len(result), "expected length mismatch")

				for i, expected := range tc.expected {
					assert.Equal(t, expected.Name, result[i].Name, "name mismatch at index %d", i)

					if expected.GetTypedConfig() != nil {
						assert.True(t, proto.Equal(expected.GetTypedConfig(), result[i].GetTypedConfig()),
							"TypedConfig mismatch at index %d\n %v\n %v\n", i, expected.GetTypedConfig(), result[i].GetTypedConfig())
					}

					// Compare Filter contents instead of pointers
					if expected.Filter != nil {
						assert.True(t, proto.Equal(expected.Filter, result[i].Filter),
							"Filter mismatch at index %d\n %v\n %v\n", i, expected.Filter, result[i].Filter)
					}
				}
			})
		}
	})
}

// Helper function to handle MessageToAny error in test cases
func mustMessageToAny(t *testing.T, msg proto.Message) *anypb.Any {
	a, err := utils.MessageToAny(msg)
	require.NoError(t, err, "failed to convert message to Any")
	return a
}
