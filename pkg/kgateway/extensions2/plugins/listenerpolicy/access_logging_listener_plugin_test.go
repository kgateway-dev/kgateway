package listenerpolicy

import (
	"context"
	"testing"
	"time"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
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
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestConvertJsonFormat_EdgeCases(t *testing.T) {
	t.Run("Access Log Conversion", func(t *testing.T) {
		testCases := []struct {
			name     string
			config   []kgateway.AccessLog
			expected []*envoyaccesslogv3.AccessLog
		}{
			{
				name:     "NilConfig",
				config:   nil,
				expected: nil,
			},
			{
				name:     "EmptyAccessLog",
				config:   []kgateway.AccessLog{},
				expected: nil,
			},
			{
				name: "FileSinkWithJSONFormat",
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path: "/var/log/access.json",
							JsonFormat: &runtime.RawExtension{
								Raw: []byte(`{"request_method": "%REQ(:METHOD)%", "response_code": "%RESPONSE_CODE%"}`),
							},
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.json",
								AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
									LogFormat: &envoycorev3.SubstitutionFormatString{
										Formatters: []*envoycorev3.TypedExtensionConfig{
											{
												Name:        "envoy.formatter.req_without_query",
												TypedConfig: mustMessageToAny(t, &envoy_req_without_query.ReqWithoutQuery{}),
											},
											{
												Name:        "envoy.formatter.metadata",
												TypedConfig: mustMessageToAny(t, &envoy_metadata_formatter.Metadata{}),
											},
										},
										Format: &envoycorev3.SubstitutionFormatString_JsonFormat{
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
				config: []kgateway.AccessLog{
					{
						GrpcService: &kgateway.AccessLogGrpcService{
							CommonAccessLogGrpcService: kgateway.CommonAccessLogGrpcService{
								CommonGrpcService: kgateway.CommonGrpcService{
									BackendRef: gwv1.BackendRef{
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
						FileSink: &kgateway.FileSink{
							Path:         "/var/log/file-access.log",
							StringFormat: new("[%START_TIME%] %RESPONSE_CODE%"),
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.http_grpc",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoygrpc.HttpGrpcAccessLogConfig{
								AdditionalRequestHeadersToLog:   []string{"x-request-id"},
								AdditionalResponseHeadersToLog:  []string{"x-response-id"},
								AdditionalResponseTrailersToLog: []string{"x-trailer"},
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									TransportApiVersion: envoycorev3.ApiVersion_V3,
									LogName:             "grpc-log",
									GrpcService: &envoycorev3.GrpcService{
										TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
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
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/file-access.log",
								AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
									LogFormat: &envoycorev3.SubstitutionFormatString{
										Formatters: []*envoycorev3.TypedExtensionConfig{
											{
												Name:        "envoy.formatter.req_without_query",
												TypedConfig: mustMessageToAny(t, &envoy_req_without_query.ReqWithoutQuery{}),
											},
											{
												Name:        "envoy.formatter.metadata",
												TypedConfig: mustMessageToAny(t, &envoy_metadata_formatter.Metadata{}),
											},
										},
										Format: &envoycorev3.SubstitutionFormatString_TextFormatSource{
											TextFormatSource: &envoycorev3.DataSource{
												Specifier: &envoycorev3.DataSource_InlineString{
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
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path:         "/var/log/access.log",
							StringFormat: new("test log format"),
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
								AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
									LogFormat: &envoycorev3.SubstitutionFormatString{
										Formatters: []*envoycorev3.TypedExtensionConfig{
											{
												Name:        "envoy.formatter.req_without_query",
												TypedConfig: mustMessageToAny(t, &envoy_req_without_query.ReqWithoutQuery{}),
											},
											{
												Name:        "envoy.formatter.metadata",
												TypedConfig: mustMessageToAny(t, &envoy_metadata_formatter.Metadata{}),
											},
										},
										Format: &envoycorev3.SubstitutionFormatString_TextFormatSource{
											TextFormatSource: &envoycorev3.DataSource{
												Specifier: &envoycorev3.DataSource_InlineString{
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
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path: "/var/log/access.log",
							JsonFormat: &runtime.RawExtension{
								Raw: []byte(`{"request_method": "%REQ(:METHOD)%", "response_code": "%RESPONSE_CODE%"}`),
							},
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
								AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
									LogFormat: &envoycorev3.SubstitutionFormatString{
										Formatters: []*envoycorev3.TypedExtensionConfig{
											{
												Name:        "envoy.formatter.req_without_query",
												TypedConfig: mustMessageToAny(t, &envoy_req_without_query.ReqWithoutQuery{}),
											},
											{
												Name:        "envoy.formatter.metadata",
												TypedConfig: mustMessageToAny(t, &envoy_metadata_formatter.Metadata{}),
											},
										},
										Format: &envoycorev3.SubstitutionFormatString_JsonFormat{
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
				config: []kgateway.AccessLog{
					{
						GrpcService: &kgateway.AccessLogGrpcService{
							CommonAccessLogGrpcService: kgateway.CommonAccessLogGrpcService{
								CommonGrpcService: kgateway.CommonGrpcService{
									BackendRef: gwv1.BackendRef{
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
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.http_grpc",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoygrpc.HttpGrpcAccessLogConfig{
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									LogName: "grpc-log",
									GrpcService: &envoycorev3.GrpcService{
										TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
												ClusterName: "backend_default_test-service_0",
											},
										},
									},
									TransportApiVersion: envoycorev3.ApiVersion_V3,
								},
							}),
						},
					},
				},
			},
			{
				name: "GrpcServiceConfig with invalid retry policy",
				config: []kgateway.AccessLog{
					{
						GrpcService: &kgateway.AccessLogGrpcService{
							CommonAccessLogGrpcService: kgateway.CommonAccessLogGrpcService{
								CommonGrpcService: kgateway.CommonGrpcService{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
										},
									},
									RetryPolicy: &kgateway.RetryPolicy{
										RetryBackOff: &kgateway.BackoffStrategy{
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
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.http_grpc",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoygrpc.HttpGrpcAccessLogConfig{
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									LogName: "grpc-log",
									GrpcService: &envoycorev3.GrpcService{
										TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
												ClusterName: "backend_default_test-service_0",
											},
										},
										RetryPolicy: &envoycorev3.RetryPolicy{
											RetryBackOff: &envoycorev3.BackoffStrategy{
												BaseInterval: &durationpb.Duration{Seconds: 5},
											},
										},
									},
									TransportApiVersion: envoycorev3.ApiVersion_V3,
								},
							}),
						},
					},
				},
			},
			{
				name: "GrpcServiceConfig with all the common options",
				config: []kgateway.AccessLog{
					{
						GrpcService: &kgateway.AccessLogGrpcService{
							CommonAccessLogGrpcService: kgateway.CommonAccessLogGrpcService{
								CommonGrpcService: kgateway.CommonGrpcService{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
										},
									},
									Authority:               new("www.example.com"),
									MaxReceiveMessageLength: new(int32(127)),
									SkipEnvoyHeaders:        new(true),
									Timeout:                 &metav1.Duration{Duration: 10 * time.Second},
									InitialMetadata: []kgateway.HeaderValue{{
										Key:   "key",
										Value: new("value"),
									}},
									RetryPolicy: &kgateway.RetryPolicy{
										RetryBackOff: &kgateway.BackoffStrategy{
											BaseInterval: metav1.Duration{Duration: 5 * time.Second},
											MaxInterval:  &metav1.Duration{Duration: 10 * time.Second},
										},
										NumRetries: new(int32(3)),
									},
								},
								LogName: "grpc-log",
							},
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.http_grpc",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoygrpc.HttpGrpcAccessLogConfig{
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									LogName: "grpc-log",
									GrpcService: &envoycorev3.GrpcService{
										TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
												ClusterName:             "backend_default_test-service_0",
												Authority:               "www.example.com",
												MaxReceiveMessageLength: &wrapperspb.UInt32Value{Value: 127},
												SkipEnvoyHeaders:        true,
											},
										},
										Timeout: &durationpb.Duration{Seconds: 10},
										InitialMetadata: []*envoycorev3.HeaderValue{{
											Key:   "key",
											Value: "value",
										}},
										RetryPolicy: &envoycorev3.RetryPolicy{
											RetryBackOff: &envoycorev3.BackoffStrategy{
												BaseInterval: &durationpb.Duration{Seconds: 5},
												MaxInterval:  &durationpb.Duration{Seconds: 10},
											},
											NumRetries: &wrapperspb.UInt32Value{Value: 3},
										},
									},
									TransportApiVersion: envoycorev3.ApiVersion_V3,
								},
							}),
						},
					},
				},
			},
			{
				name: "AccessLogWithStatusCodeFilter",
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path:         "/var/log/access.log",
							StringFormat: new("hello kgateway"),
						},
						Filter: &kgateway.AccessLogFilter{
							FilterType: &kgateway.FilterType{
								StatusCodeFilter: &kgateway.StatusCodeFilter{
									Op:    kgateway.EQ,
									Value: 5,
								},
							},
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
								AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
									LogFormat: &envoycorev3.SubstitutionFormatString{
										Formatters: []*envoycorev3.TypedExtensionConfig{
											{
												Name:        "envoy.formatter.req_without_query",
												TypedConfig: mustMessageToAny(t, &envoy_req_without_query.ReqWithoutQuery{}),
											},
											{
												Name:        "envoy.formatter.metadata",
												TypedConfig: mustMessageToAny(t, &envoy_metadata_formatter.Metadata{}),
											},
										},
										Format: &envoycorev3.SubstitutionFormatString_TextFormatSource{
											TextFormatSource: &envoycorev3.DataSource{
												Specifier: &envoycorev3.DataSource_InlineString{
													InlineString: "hello kgateway",
												},
											},
										},
									},
								},
							}),
						},
						Filter: &envoyaccesslogv3.AccessLogFilter{
							FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_StatusCodeFilter{
								StatusCodeFilter: &envoyaccesslogv3.StatusCodeFilter{
									Comparison: &envoyaccesslogv3.ComparisonFilter{
										Op: envoyaccesslogv3.ComparisonFilter_EQ,
										Value: &envoycorev3.RuntimeUInt32{
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
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &kgateway.AccessLogFilter{
							FilterType: &kgateway.FilterType{
								HeaderFilter: &kgateway.HeaderFilter{
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
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &envoyaccesslogv3.AccessLogFilter{
							FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_HeaderFilter{
								HeaderFilter: &envoyaccesslogv3.HeaderFilter{
									Header: &envoyroutev3.HeaderMatcher{
										Name: "x-test-header",
										HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_StringMatch{
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
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &kgateway.AccessLogFilter{
							FilterType: &kgateway.FilterType{
								DurationFilter: &kgateway.DurationFilter{
									Op:    kgateway.EQ,
									Value: 5,
								},
							},
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &envoyaccesslogv3.AccessLogFilter{
							FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_DurationFilter{
								DurationFilter: &envoyaccesslogv3.DurationFilter{
									Comparison: &envoyaccesslogv3.ComparisonFilter{
										Op: envoyaccesslogv3.ComparisonFilter_EQ,
										Value: &envoycorev3.RuntimeUInt32{
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
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &kgateway.AccessLogFilter{
							FilterType: &kgateway.FilterType{
								NotHealthCheckFilter: new(true),
							},
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &envoyaccesslogv3.AccessLogFilter{
							FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_NotHealthCheckFilter{
								NotHealthCheckFilter: &envoyaccesslogv3.NotHealthCheckFilter{},
							},
						},
					},
				},
			},
			{
				name: "TraceableFilter",
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &kgateway.AccessLogFilter{
							FilterType: &kgateway.FilterType{
								TraceableFilter: new(true),
							},
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &envoyaccesslogv3.AccessLogFilter{
							FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_TraceableFilter{
								TraceableFilter: &envoyaccesslogv3.TraceableFilter{},
							},
						},
					},
				},
			},
			{
				name: "ResponseFlagFilter",
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &kgateway.AccessLogFilter{
							FilterType: &kgateway.FilterType{
								ResponseFlagFilter: &kgateway.ResponseFlagFilter{
									Flags: []string{
										"test-flag",
									},
								},
							},
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &envoyaccesslogv3.AccessLogFilter{
							FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_ResponseFlagFilter{
								ResponseFlagFilter: &envoyaccesslogv3.ResponseFlagFilter{
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
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &kgateway.AccessLogFilter{
							FilterType: &kgateway.FilterType{
								GrpcStatusFilter: &kgateway.GrpcStatusFilter{
									Statuses: []kgateway.GrpcStatus{kgateway.NOT_FOUND},
								},
							},
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &envoyaccesslogv3.AccessLogFilter{
							FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_GrpcStatusFilter{
								GrpcStatusFilter: &envoyaccesslogv3.GrpcStatusFilter{
									Statuses: []envoyaccesslogv3.GrpcStatusFilter_Status{envoyaccesslogv3.GrpcStatusFilter_NOT_FOUND},
								},
							},
						},
					},
				},
			},
			{
				name: "CELFilter",
				config: []kgateway.AccessLog{
					{
						FileSink: &kgateway.FileSink{
							Path: "/var/log/access.log",
						},
						Filter: &kgateway.AccessLogFilter{
							FilterType: &kgateway.FilterType{
								CELFilter: &kgateway.CELFilter{
									Match: "connection.mtls",
								},
							},
						},
					},
				},
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.file",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoyalfile.FileAccessLog{
								Path: "/var/log/access.log",
							}),
						},
						Filter: &envoyaccesslogv3.AccessLogFilter{
							FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_ExtensionFilter{
								ExtensionFilter: &envoyaccesslogv3.ExtensionFilter{
									Name: wellknown.CELExtensionFilter,
									ConfigType: &envoyaccesslogv3.ExtensionFilter_TypedConfig{
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
				config: []kgateway.AccessLog{
					{
						OpenTelemetry: &kgateway.OpenTelemetryAccessLogService{
							GrpcService: kgateway.CommonAccessLogGrpcService{
								CommonGrpcService: kgateway.CommonGrpcService{
									BackendRef: gwv1.BackendRef{
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
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.open_telemetry",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoy_open_telemetry.OpenTelemetryAccessLogConfig{
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									LogName: "otel-log",
									GrpcService: &envoycorev3.GrpcService{
										TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
												ClusterName: "backend_default_test-service_0",
											},
										},
									},
									TransportApiVersion: envoycorev3.ApiVersion_V3,
								},
								ResourceAttributes: &otelv1.KeyValueList{
									Values: []*otelv1.KeyValue{
										{
											Key: "service.name",
											Value: &otelv1.AnyValue{
												Value: &otelv1.AnyValue_StringValue{
													StringValue: "gw.default",
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
				name: "OTel Sink with all the options",
				config: []kgateway.AccessLog{
					{
						OpenTelemetry: &kgateway.OpenTelemetryAccessLogService{
							GrpcService: kgateway.CommonAccessLogGrpcService{
								CommonGrpcService: kgateway.CommonGrpcService{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
										},
									},
								},
								LogName: "otel-log",
							},
							Body:                 new(`"%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %RESPONSE_CODE% "%REQ(:AUTHORITY)%" "%UPSTREAM_CLUSTER%"\n'`),
							DisableBuiltinLabels: new(true),
							ResourceAttributes: &kgateway.KeyAnyValueList{
								Values: []kgateway.KeyAnyValue{
									{
										Key: "ra-string-key-1",
										Value: kgateway.AnyValue{
											StringValue: new("ra-string-value-1"),
										},
									},
									{
										Key: "service.name",
										Value: kgateway.AnyValue{
											StringValue: new("my:service"),
										},
									},
									{
										Key: "ra-array-key",
										Value: kgateway.AnyValue{
											ArrayValue: []kgateway.AnyValue{
												{
													StringValue: new("ra-1-string-value"),
												},
												{
													StringValue: new("ra-2-string-value"),
												},
											},
										},
									},
									{
										Key: "ra-kvlist-key",
										Value: kgateway.AnyValue{
											KvListValue: &kgateway.KeyAnyValueList{
												Values: []kgateway.KeyAnyValue{
													{
														Key: "ra-string-key-2",
														Value: kgateway.AnyValue{
															StringValue: new("ra-string-value-2"),
														},
													},
													{
														Key: "ra-array-key",
														Value: kgateway.AnyValue{
															ArrayValue: []kgateway.AnyValue{
																{
																	StringValue: new("ra-3-string-value"),
																},
																{
																	StringValue: new("ra-4-string-value"),
																},
															},
														},
													},
													{
														Key: "ra-kvlist-key",
														Value: kgateway.AnyValue{
															KvListValue: &kgateway.KeyAnyValueList{
																Values: []kgateway.KeyAnyValue{
																	{
																		Key: "ra-string-key-3",
																		Value: kgateway.AnyValue{
																			StringValue: new("ra-string-value-3"),
																		},
																	},
																	{
																		Key: "ra-string-key-4",
																		Value: kgateway.AnyValue{
																			StringValue: new("ra-string-value-4"),
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
							Attributes: &kgateway.KeyAnyValueList{
								Values: []kgateway.KeyAnyValue{
									{
										Key: "string-key-1",
										Value: kgateway.AnyValue{
											StringValue: new("string-value-1"),
										},
									},
									{
										Key: "array-key",
										Value: kgateway.AnyValue{
											ArrayValue: []kgateway.AnyValue{
												{
													StringValue: new("1-string-value"),
												},
												{
													StringValue: new("2-string-value"),
												},
											},
										},
									},
									{
										Key: "kvlist-key",
										Value: kgateway.AnyValue{
											KvListValue: &kgateway.KeyAnyValueList{
												Values: []kgateway.KeyAnyValue{
													{
														Key: "string-key-2",
														Value: kgateway.AnyValue{
															StringValue: new("string-value-2"),
														},
													},
													{
														Key: "array-key",
														Value: kgateway.AnyValue{
															ArrayValue: []kgateway.AnyValue{
																{
																	StringValue: new("3-string-value"),
																},
																{
																	StringValue: new("4-string-value"),
																},
															},
														},
													},
													{
														Key: "kvlist-key",
														Value: kgateway.AnyValue{
															KvListValue: &kgateway.KeyAnyValueList{
																Values: []kgateway.KeyAnyValue{
																	{
																		Key: "string-key-3",
																		Value: kgateway.AnyValue{
																			StringValue: new("string-value-3"),
																		},
																	},
																	{
																		Key: "string-key-4",
																		Value: kgateway.AnyValue{
																			StringValue: new("string-value-4"),
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
				expected: []*envoyaccesslogv3.AccessLog{
					{
						Name: "envoy.access_loggers.open_telemetry",
						ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
							TypedConfig: mustMessageToAny(t, &envoy_open_telemetry.OpenTelemetryAccessLogConfig{
								CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
									LogName: "otel-log",
									GrpcService: &envoycorev3.GrpcService{
										TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
												ClusterName: "backend_default_test-service_0",
											},
										},
									},
									TransportApiVersion: envoycorev3.ApiVersion_V3,
								},
								Body: &otelv1.AnyValue{
									Value: &otelv1.AnyValue_StringValue{
										StringValue: `"%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %RESPONSE_CODE% "%REQ(:AUTHORITY)%" "%UPSTREAM_CLUSTER%"\n'`,
									},
								},
								DisableBuiltinLabels: true,
								ResourceAttributes: &otelv1.KeyValueList{
									Values: []*otelv1.KeyValue{
										{
											Key: "ra-string-key-1",
											Value: &otelv1.AnyValue{
												Value: &otelv1.AnyValue_StringValue{
													StringValue: "ra-string-value-1",
												},
											},
										},
										{
											Key: "service.name",
											Value: &otelv1.AnyValue{
												Value: &otelv1.AnyValue_StringValue{
													StringValue: "my:service",
												},
											},
										},
										{
											Key: "ra-array-key",
											Value: &otelv1.AnyValue{
												Value: &otelv1.AnyValue_ArrayValue{
													ArrayValue: &otelv1.ArrayValue{
														Values: []*otelv1.AnyValue{
															{
																Value: &otelv1.AnyValue_StringValue{
																	StringValue: "ra-1-string-value",
																},
															},
															{
																Value: &otelv1.AnyValue_StringValue{
																	StringValue: "ra-2-string-value",
																},
															},
														},
													},
												},
											},
										},
										{
											Key: "ra-kvlist-key",
											Value: &otelv1.AnyValue{
												Value: &otelv1.AnyValue_KvlistValue{
													KvlistValue: &otelv1.KeyValueList{
														Values: []*otelv1.KeyValue{
															{
																Key: "ra-string-key-2",
																Value: &otelv1.AnyValue{
																	Value: &otelv1.AnyValue_StringValue{
																		StringValue: "ra-string-value-2",
																	},
																},
															},
															{
																Key: "ra-array-key",
																Value: &otelv1.AnyValue{
																	Value: &otelv1.AnyValue_ArrayValue{
																		ArrayValue: &otelv1.ArrayValue{
																			Values: []*otelv1.AnyValue{
																				{
																					Value: &otelv1.AnyValue_StringValue{
																						StringValue: "ra-3-string-value",
																					},
																				},
																				{
																					Value: &otelv1.AnyValue_StringValue{
																						StringValue: "ra-4-string-value",
																					},
																				},
																			},
																		},
																	},
																},
															},
															{
																Key: "ra-kvlist-key",
																Value: &otelv1.AnyValue{
																	Value: &otelv1.AnyValue_KvlistValue{
																		KvlistValue: &otelv1.KeyValueList{
																			Values: []*otelv1.KeyValue{
																				{
																					Key: "ra-string-key-3",
																					Value: &otelv1.AnyValue{
																						Value: &otelv1.AnyValue_StringValue{
																							StringValue: "ra-string-value-3",
																						},
																					},
																				},
																				{
																					Key: "ra-string-key-4",
																					Value: &otelv1.AnyValue{
																						Value: &otelv1.AnyValue_StringValue{
																							StringValue: "ra-string-value-4",
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
				configs, err := translateAccessLogs(
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
				result, err := generateAccessLogConfig(&ir.HcmContext{
					Gateway: ir.GatewayIR{
						SourceObject: &ir.Gateway{
							ObjectSource: ir.ObjectSource{
								Namespace: "default",
								Name:      "gw",
							},
						},
					},
				}, tc.config, configs)
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

func TestAccessLogFilters(t *testing.T) {
	type verifyFn func(t *testing.T, got *envoyaccesslogv3.AccessLog)

	headerExact := gwv1.HeaderMatchExact

	tests := []struct {
		name     string
		alFilter *kgateway.AccessLogFilter
		verify   verifyFn
	}{
		{
			name: "StatusCode GE 400",
			alFilter: &kgateway.AccessLogFilter{
				FilterType: &kgateway.FilterType{
					StatusCodeFilter: &kgateway.StatusCodeFilter{
						Op:    kgateway.GE,
						Value: 400,
					},
				},
			},
			verify: func(t *testing.T, got *envoyaccesslogv3.AccessLog) {
				sc := got.GetFilter().GetStatusCodeFilter()
				require.NotNil(t, sc)
				require.NotNil(t, sc.Comparison)
				assert.Equal(t, envoyaccesslogv3.ComparisonFilter_GE, sc.Comparison.Op)
				assert.Equal(t, uint32(400), sc.GetComparison().GetValue().GetDefaultValue())
			},
		},
		{
			name: "Duration LE 10",
			alFilter: &kgateway.AccessLogFilter{
				FilterType: &kgateway.FilterType{
					DurationFilter: &kgateway.DurationFilter{
						Op:    kgateway.LE,
						Value: 10,
					},
				},
			},
			verify: func(t *testing.T, got *envoyaccesslogv3.AccessLog) {
				df := got.GetFilter().GetDurationFilter()
				require.NotNil(t, df)
				require.NotNil(t, df.Comparison)
				assert.Equal(t, envoyaccesslogv3.ComparisonFilter_LE, df.Comparison.Op)
				assert.Equal(t, uint32(10), df.GetComparison().GetValue().GetDefaultValue())
			},
		},
		{
			name: "NotHealthCheck",
			alFilter: &kgateway.AccessLogFilter{
				FilterType: &kgateway.FilterType{NotHealthCheckFilter: new(true)},
			},
			verify: func(t *testing.T, got *envoyaccesslogv3.AccessLog) {
				require.NotNil(t, got.GetFilter().GetNotHealthCheckFilter())
			},
		},
		{
			name: "Traceable",
			alFilter: &kgateway.AccessLogFilter{
				FilterType: &kgateway.FilterType{TraceableFilter: new(true)},
			},
			verify: func(t *testing.T, got *envoyaccesslogv3.AccessLog) {
				require.NotNil(t, got.GetFilter().GetTraceableFilter())
			},
		},
		{
			name: "Header Exact",
			alFilter: &kgateway.AccessLogFilter{FilterType: &kgateway.FilterType{
				HeaderFilter: &kgateway.HeaderFilter{
					Header: gwv1.HTTPHeaderMatch{Type: &headerExact, Name: gwv1.HTTPHeaderName("x-test"), Value: "val"},
				},
			}},
			verify: func(t *testing.T, got *envoyaccesslogv3.AccessLog) {
				hf := got.GetFilter().GetHeaderFilter()
				require.NotNil(t, hf)
				require.NotNil(t, hf.Header)
				assert.Equal(t, "x-test", hf.Header.GetName())
				sm := hf.Header.GetStringMatch()
				require.NotNil(t, sm)
				assert.Equal(t, "val", sm.GetExact())
			},
		},
		{
			name: "ResponseFlag UH",
			alFilter: &kgateway.AccessLogFilter{
				FilterType: &kgateway.FilterType{
					ResponseFlagFilter: &kgateway.ResponseFlagFilter{
						Flags: []string{"UH"},
					},
				},
			},
			verify: func(t *testing.T, got *envoyaccesslogv3.AccessLog) {
				rf := got.GetFilter().GetResponseFlagFilter()
				require.NotNil(t, rf)
				assert.Contains(t, rf.Flags, "UH")
			},
		},
		{
			name: "GrpcStatus NOT_FOUND",
			alFilter: &kgateway.AccessLogFilter{
				FilterType: &kgateway.FilterType{
					GrpcStatusFilter: &kgateway.GrpcStatusFilter{
						Statuses: []kgateway.GrpcStatus{kgateway.NOT_FOUND},
					},
				},
			},
			verify: func(t *testing.T, got *envoyaccesslogv3.AccessLog) {
				gs := got.GetFilter().GetGrpcStatusFilter()
				require.NotNil(t, gs)
				require.Len(t, gs.Statuses, 1)
				assert.Equal(t, envoyaccesslogv3.GrpcStatusFilter_NOT_FOUND, gs.Statuses[0])
			},
		},
		{
			name: "CEL",
			alFilter: &kgateway.AccessLogFilter{
				FilterType: &kgateway.FilterType{
					CELFilter: &kgateway.CELFilter{Match: "response.code >= 400"},
				},
			},
			verify: func(t *testing.T, got *envoyaccesslogv3.AccessLog) {
				ext := got.GetFilter().GetExtensionFilter()
				require.NotNil(t, ext)
				assert.Equal(t, wellknown.CELExtensionFilter, ext.GetName())
			},
		},
		{
			name: "And NotHealthCheck && Traceable",
			alFilter: &kgateway.AccessLogFilter{
				AndFilter: []kgateway.FilterType{{NotHealthCheckFilter: new(true)}, {TraceableFilter: new(true)}},
			},
			verify: func(t *testing.T, got *envoyaccesslogv3.AccessLog) {
				and := got.GetFilter().GetAndFilter()
				require.NotNil(t, and)
				require.Len(t, and.Filters, 2)
				assert.NotNil(t, and.Filters[0].GetNotHealthCheckFilter())
				assert.NotNil(t, and.Filters[1].GetTraceableFilter())
			},
		},
		{
			name: "Or Header || ResponseFlag",
			alFilter: &kgateway.AccessLogFilter{
				OrFilter: []kgateway.FilterType{
					{
						HeaderFilter: &kgateway.HeaderFilter{
							Header: gwv1.HTTPHeaderMatch{Type: &headerExact, Name: gwv1.HTTPHeaderName("x-test"), Value: "val"},
						},
					},
					{
						ResponseFlagFilter: &kgateway.ResponseFlagFilter{Flags: []string{"UH"}},
					},
				},
			},
			verify: func(t *testing.T, got *envoyaccesslogv3.AccessLog) {
				orf := got.GetFilter().GetOrFilter()
				require.NotNil(t, orf)
				require.Len(t, orf.Filters, 2)
				assert.NotNil(t, orf.Filters[0].GetHeaderFilter())
				assert.NotNil(t, orf.Filters[1].GetResponseFlagFilter())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfgs, err := translateAccessLogs([]kgateway.AccessLog{{
				FileSink: &kgateway.FileSink{Path: "/dev/stdout"},
				Filter:   tc.alFilter,
			}}, nil)
			require.NoError(t, err)

			hcmCtx := &ir.HcmContext{
				Gateway: ir.GatewayIR{
					SourceObject: &ir.Gateway{
						ObjectSource: ir.ObjectSource{
							Name:      "gw",
							Namespace: "default",
						},
					},
				},
			}

			accessLogs := []kgateway.AccessLog{{
				Filter:   tc.alFilter,
				FileSink: &kgateway.FileSink{Path: "/dev/stdout"},
			}}

			got, err := generateAccessLogConfig(hcmCtx, accessLogs, cfgs)
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.NotNil(t, got[0].GetFilter())
			tc.verify(t, got[0])
		})
	}
}

// Helper function to handle MessageToAny error in test cases
func mustMessageToAny(t *testing.T, msg proto.Message) *anypb.Any {
	a, err := utils.MessageToAny(msg)
	require.NoError(t, err, "failed to convert message to Any")
	return a
}
