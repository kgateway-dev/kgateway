package als_test

import (
	"strconv"

	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_types "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloo_envoy_v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	gloo_envoy_route "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/route/v3"
	gloo_envoy_types "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	. "github.com/onsi/ginkgo/extensions/table"
	accessLogService "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/test/matchers"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoygrpc "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
)

var _ = Describe("Plugin", func() {

	const (
		FILTER_RUNTIME_KEY                                 = "FILTER RUNTIME KEY"
		STATUS_CODE_VALUE                           uint32 = 400
		DURATION_FILTER_VALUE                       uint32 = 20
		FRACTIONAL_PERCENT_NUMERATOR                uint32 = 25
		FRACTIONAL_PERCENT_DENOMINATOR_TYPE         uint32 = 1
		INVALID_FRACTIONAL_PERCENT_DENOMINATOR_TYPE uint32 = 10
		INVALID_OP                                         = 10
		HEADER_MATCHER_NAME_STRING                         = "HEADER MATCHER NAME STRING"
	)

	// Fake constant for the slice
	var response_flags_test_constant = []string{"LH", "UH", "UT"}

	// Because we are just translatating the filters using marshaling/unmarshaling, we should test each filter type
	// to make sure we copied/pasted correctly and that no changes made to the Envoy definitions broke anything
	Describe("Test each Filter", func() {
		var (
			alsSettings      *accessLogService.AccessLoggingService
			logName          string
			extraHeaders     []string
			usRef            *core.ResourceRef
			accessLogConfigs []*envoyal.AccessLog
			err              error
		)

		BeforeEach(func() {
			logName = "test"
			extraHeaders = []string{"test"}
			usRef = &core.ResourceRef{
				Name:      "default",
				Namespace: "default",
			}

			alsSettings = &accessLogService.AccessLoggingService{
				AccessLog: []*accessLogService.AccessLog{
					{
						OutputDestination: &accessLogService.AccessLog_GrpcService{
							GrpcService: &accessLogService.GrpcService{
								LogName: logName,
								ServiceRef: &accessLogService.GrpcService_StaticClusterName{
									StaticClusterName: translatorutil.UpstreamToClusterName(usRef),
								},
								AdditionalRequestHeadersToLog:   extraHeaders,
								AdditionalResponseHeadersToLog:  extraHeaders,
								AdditionalResponseTrailersToLog: extraHeaders,
							},
						},
					},
				},
			}
		})

		Describe("Test each Filter", func() {
			AfterEach(func() {
				accessLogConfig := accessLogConfigs[0]

				Expect(accessLogConfig.Name).To(Equal(wellknown.HTTPGRPCAccessLog))
				var falCfg envoygrpc.HttpGrpcAccessLogConfig
				err = translatorutil.ParseTypedConfig(accessLogConfig, &falCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(falCfg.AdditionalRequestHeadersToLog).To(Equal(extraHeaders))
				Expect(falCfg.AdditionalResponseHeadersToLog).To(Equal(extraHeaders))
				Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
				Expect(falCfg.CommonConfig.LogName).To(Equal(logName))
				envoyGrpc := falCfg.CommonConfig.GetGrpcService().GetEnvoyGrpc()
				Expect(envoyGrpc).NotTo(BeNil())
				Expect(envoyGrpc.ClusterName).To(Equal(translatorutil.UpstreamToClusterName(usRef)))
			})

			DescribeTable("Test each filter is translated properly",
				func(glooInputFilter *accessLogService.AccessLogFilter, expectedEnvoyFilter *envoyal.AccessLogFilter) {

					accessLog := alsSettings.GetAccessLog()[0]
					accessLog.Filter = glooInputFilter

					accessLogConfigs, err = ProcessAccessLogPlugins(alsSettings, nil)
					Expect(err).NotTo(HaveOccurred())

					Expect(accessLogConfigs).To(HaveLen(1))
					accessLogConfig := accessLogConfigs[0]

					accessLogFilter := accessLogConfig.GetFilter()
					Expect(accessLogFilter).To(matchers.MatchProto(expectedEnvoyFilter))

				},
				Entry(
					"nil filter",
					&accessLogService.AccessLogFilter{},
					&envoyal.AccessLogFilter{}),
				Entry(
					"StatusCodeFilter",
					&accessLogService.AccessLogFilter{
						FilterSpecifier: &accessLogService.AccessLogFilter_StatusCodeFilter{
							StatusCodeFilter: &accessLogService.StatusCodeFilter{
								Comparison: &accessLogService.ComparisonFilter{
									Op: accessLogService.ComparisonFilter_EQ,
									Value: &gloo_envoy_v3.RuntimeUInt32{
										DefaultValue: STATUS_CODE_VALUE,
										RuntimeKey:   FILTER_RUNTIME_KEY,
									},
								},
							},
						},
					},
					&envoyal.AccessLogFilter{
						FilterSpecifier: &envoyal.AccessLogFilter_StatusCodeFilter{
							StatusCodeFilter: &envoyal.StatusCodeFilter{
								Comparison: &envoyal.ComparisonFilter{
									Op: envoyal.ComparisonFilter_EQ,
									Value: &envoy_v3.RuntimeUInt32{
										DefaultValue: STATUS_CODE_VALUE,
										RuntimeKey:   FILTER_RUNTIME_KEY,
									},
								},
							},
						},
					}),
				Entry(
					"DurationFilter",
					&accessLogService.AccessLogFilter{
						FilterSpecifier: &accessLogService.AccessLogFilter_DurationFilter{
							DurationFilter: &accessLogService.DurationFilter{
								Comparison: &accessLogService.ComparisonFilter{
									Op: accessLogService.ComparisonFilter_EQ,
									Value: &gloo_envoy_v3.RuntimeUInt32{
										DefaultValue: DURATION_FILTER_VALUE,
										RuntimeKey:   FILTER_RUNTIME_KEY,
									},
								},
							},
						},
					},
					&envoyal.AccessLogFilter{
						FilterSpecifier: &envoyal.AccessLogFilter_DurationFilter{
							DurationFilter: &envoyal.DurationFilter{
								Comparison: &envoyal.ComparisonFilter{
									Op: envoyal.ComparisonFilter_EQ,
									Value: &envoy_v3.RuntimeUInt32{
										DefaultValue: DURATION_FILTER_VALUE,
										RuntimeKey:   FILTER_RUNTIME_KEY,
									},
								},
							},
						},
					}),
				Entry(
					"NotHealthCheckFilter",
					&accessLogService.AccessLogFilter{
						FilterSpecifier: &accessLogService.AccessLogFilter_NotHealthCheckFilter{
							NotHealthCheckFilter: &accessLogService.NotHealthCheckFilter{},
						},
					},
					&envoyal.AccessLogFilter{
						FilterSpecifier: &envoyal.AccessLogFilter_NotHealthCheckFilter{
							NotHealthCheckFilter: &envoyal.NotHealthCheckFilter{},
						},
					}),
				Entry(
					"TraceableFilter",
					&accessLogService.AccessLogFilter{
						FilterSpecifier: &accessLogService.AccessLogFilter_TraceableFilter{
							TraceableFilter: &accessLogService.TraceableFilter{},
						},
					},
					&envoyal.AccessLogFilter{
						FilterSpecifier: &envoyal.AccessLogFilter_TraceableFilter{
							TraceableFilter: &envoyal.TraceableFilter{},
						},
					}),
				Entry(
					"RuntimeFilter",
					&accessLogService.AccessLogFilter{
						FilterSpecifier: &accessLogService.AccessLogFilter_RuntimeFilter{
							RuntimeFilter: &accessLogService.RuntimeFilter{
								RuntimeKey: FILTER_RUNTIME_KEY,
								PercentSampled: &gloo_envoy_types.FractionalPercent{
									Numerator:   FRACTIONAL_PERCENT_NUMERATOR,
									Denominator: gloo_envoy_types.FractionalPercent_DenominatorType(FRACTIONAL_PERCENT_DENOMINATOR_TYPE),
								},
								UseIndependentRandomness: true,
							},
						},
					},
					&envoyal.AccessLogFilter{
						FilterSpecifier: &envoyal.AccessLogFilter_RuntimeFilter{
							RuntimeFilter: &envoyal.RuntimeFilter{
								RuntimeKey: FILTER_RUNTIME_KEY,
								PercentSampled: &envoy_types.FractionalPercent{
									Numerator:   FRACTIONAL_PERCENT_NUMERATOR,
									Denominator: envoy_types.FractionalPercent_DenominatorType(FRACTIONAL_PERCENT_DENOMINATOR_TYPE),
								},
								UseIndependentRandomness: true,
							},
						},
					}),
				Entry(
					"AndFilter",
					&accessLogService.AccessLogFilter{
						FilterSpecifier: &accessLogService.AccessLogFilter_AndFilter{
							AndFilter: &accessLogService.AndFilter{
								Filters: []*accessLogService.AccessLogFilter{
									{
										FilterSpecifier: &accessLogService.AccessLogFilter_HeaderFilter{},
									},
									{
										FilterSpecifier: &accessLogService.AccessLogFilter_ResponseFlagFilter{},
									},
									{
										FilterSpecifier: &accessLogService.AccessLogFilter_GrpcStatusFilter{},
									},
								},
							},
						},
					},
					&envoyal.AccessLogFilter{
						FilterSpecifier: &envoyal.AccessLogFilter_AndFilter{
							AndFilter: &envoyal.AndFilter{
								Filters: []*envoyal.AccessLogFilter{
									{
										FilterSpecifier: &envoyal.AccessLogFilter_HeaderFilter{},
									},
									{
										FilterSpecifier: &envoyal.AccessLogFilter_ResponseFlagFilter{},
									},
									{
										FilterSpecifier: &envoyal.AccessLogFilter_GrpcStatusFilter{},
									},
								},
							},
						},
					}),
				Entry(
					"OrFilter",
					&accessLogService.AccessLogFilter{
						FilterSpecifier: &accessLogService.AccessLogFilter_OrFilter{
							OrFilter: &accessLogService.OrFilter{
								Filters: []*accessLogService.AccessLogFilter{
									{
										FilterSpecifier: &accessLogService.AccessLogFilter_HeaderFilter{},
									},
									{
										FilterSpecifier: &accessLogService.AccessLogFilter_ResponseFlagFilter{},
									},
									{
										FilterSpecifier: &accessLogService.AccessLogFilter_GrpcStatusFilter{},
									},
								},
							},
						},
					},
					&envoyal.AccessLogFilter{
						FilterSpecifier: &envoyal.AccessLogFilter_OrFilter{
							OrFilter: &envoyal.OrFilter{
								Filters: []*envoyal.AccessLogFilter{
									{
										FilterSpecifier: &envoyal.AccessLogFilter_HeaderFilter{},
									},
									{
										FilterSpecifier: &envoyal.AccessLogFilter_ResponseFlagFilter{},
									},
									{
										FilterSpecifier: &envoyal.AccessLogFilter_GrpcStatusFilter{},
									},
								},
							},
						},
					}),
				Entry(
					"HeaderFilter",
					&accessLogService.AccessLogFilter{
						FilterSpecifier: &accessLogService.AccessLogFilter_HeaderFilter{
							HeaderFilter: &accessLogService.HeaderFilter{
								Header: &gloo_envoy_route.HeaderMatcher{
									Name:        HEADER_MATCHER_NAME_STRING,
									InvertMatch: true,
								},
							},
						},
					},
					&envoyal.AccessLogFilter{
						FilterSpecifier: &envoyal.AccessLogFilter_HeaderFilter{
							HeaderFilter: &envoyal.HeaderFilter{
								Header: &envoy_route.HeaderMatcher{
									Name:        HEADER_MATCHER_NAME_STRING,
									InvertMatch: true,
								},
							},
						},
					}),
				Entry(
					"ResponseFlagFilter",
					&accessLogService.AccessLogFilter{
						FilterSpecifier: &accessLogService.AccessLogFilter_ResponseFlagFilter{
							ResponseFlagFilter: &accessLogService.ResponseFlagFilter{
								Flags: response_flags_test_constant,
							},
						},
					},
					&envoyal.AccessLogFilter{
						FilterSpecifier: &envoyal.AccessLogFilter_ResponseFlagFilter{
							ResponseFlagFilter: &envoyal.ResponseFlagFilter{
								Flags: response_flags_test_constant,
							},
						},
					}),
				Entry(
					"GrpcStatusFilter",
					&accessLogService.AccessLogFilter{
						FilterSpecifier: &accessLogService.AccessLogFilter_GrpcStatusFilter{
							GrpcStatusFilter: &accessLogService.GrpcStatusFilter{
								// We're using CONSTANTS elsewhere, but its easier to just put the values directly
								// into the literal slice, especially since the gloo/envoy types are technically different
								Statuses: []accessLogService.GrpcStatusFilter_Status{1, 2},
								Exclude:  false,
							},
						},
					},
					&envoyal.AccessLogFilter{
						FilterSpecifier: &envoyal.AccessLogFilter_GrpcStatusFilter{
							GrpcStatusFilter: &envoyal.GrpcStatusFilter{
								Statuses: []envoyal.GrpcStatusFilter_Status{1, 2},
								Exclude:  false,
							},
						},
					}),
			)

		})

		DescribeTable("Test We Correctly Handle Bad Enum",
			func(glooInputFilter *accessLogService.AccessLogFilter, expectedError error) {

				accessLog := alsSettings.GetAccessLog()[0]
				accessLog.Filter = glooInputFilter

				accessLogConfigs, err = ProcessAccessLogPlugins(alsSettings, nil)
				Expect(err).To(HaveOccurred())
				Expect(err).Should(MatchError(expectedError))

				Expect(accessLogConfigs).To(HaveLen(0))
			},
			Entry(
				"Bad Denominator in RuntimeFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_RuntimeFilter{
						RuntimeFilter: &accessLogService.RuntimeFilter{
							RuntimeKey: FILTER_RUNTIME_KEY,
							PercentSampled: &gloo_envoy_types.FractionalPercent{
								Numerator:   FRACTIONAL_PERCENT_NUMERATOR,
								Denominator: gloo_envoy_types.FractionalPercent_DenominatorType(INVALID_FRACTIONAL_PERCENT_DENOMINATOR_TYPE),
							},
							UseIndependentRandomness: true,
						},
					},
				},
				InvalidEnumValueError("RuntimeFilter", "FractionalPercent.Denominator", strconv.FormatUint(uint64(INVALID_FRACTIONAL_PERCENT_DENOMINATOR_TYPE), 10)),
			),
			Entry(
				"Missing RuntimeKey in RuntimeFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_RuntimeFilter{
						RuntimeFilter: &accessLogService.RuntimeFilter{
							PercentSampled: &gloo_envoy_types.FractionalPercent{
								Numerator:   FRACTIONAL_PERCENT_NUMERATOR,
								Denominator: gloo_envoy_types.FractionalPercent_DenominatorType(FRACTIONAL_PERCENT_DENOMINATOR_TYPE),
							},
							UseIndependentRandomness: true,
						},
					},
				},
				NoValueError("RuntimeFilter", "FractionalPercent.RuntimeKey"),
			),
			Entry(
				"Bad OP in StatusCodeFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_StatusCodeFilter{
						StatusCodeFilter: &accessLogService.StatusCodeFilter{
							Comparison: &accessLogService.ComparisonFilter{
								Op: accessLogService.ComparisonFilter_Op(INVALID_OP),
								Value: &gloo_envoy_v3.RuntimeUInt32{
									DefaultValue: STATUS_CODE_VALUE,
									RuntimeKey:   FILTER_RUNTIME_KEY,
								},
							},
						},
					},
				},
				InvalidEnumValueError("StatusCodeFilter", "ComparisonFilter.Op", strconv.Itoa(INVALID_OP)),
			),
			Entry(
				"Missing Value in StatusCodeFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_StatusCodeFilter{
						StatusCodeFilter: &accessLogService.StatusCodeFilter{
							Comparison: &accessLogService.ComparisonFilter{
								Op: accessLogService.ComparisonFilter_EQ,
							},
						},
					},
				},
				NoValueError("StatusCodeFilter", "ComparisonFilter.Value"),
			),
			Entry(
				"Missing RuntimeKey in StatusCodeFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_StatusCodeFilter{
						StatusCodeFilter: &accessLogService.StatusCodeFilter{
							Comparison: &accessLogService.ComparisonFilter{
								Op: accessLogService.ComparisonFilter_EQ,
								Value: &gloo_envoy_v3.RuntimeUInt32{
									DefaultValue: STATUS_CODE_VALUE,
								},
							},
						},
					},
				},
				NoValueError("StatusCodeFilter", "ComparisonFilter.Value.RuntimeKey"),
			),
			Entry(
				"Bad OP in DurationFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_DurationFilter{
						DurationFilter: &accessLogService.DurationFilter{
							Comparison: &accessLogService.ComparisonFilter{
								Op: accessLogService.ComparisonFilter_Op(INVALID_OP),
								Value: &gloo_envoy_v3.RuntimeUInt32{
									DefaultValue: STATUS_CODE_VALUE,
									RuntimeKey:   FILTER_RUNTIME_KEY,
								},
							},
						},
					},
				},
				InvalidEnumValueError("DurationFilter", "ComparisonFilter.Op", strconv.Itoa(INVALID_OP)),
			),
			Entry(
				"Missing Value in DurationFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_DurationFilter{
						DurationFilter: &accessLogService.DurationFilter{
							Comparison: &accessLogService.ComparisonFilter{
								Op: accessLogService.ComparisonFilter_EQ,
							},
						},
					},
				},
				NoValueError("DurationFilter", "ComparisonFilter.Value"),
			),
			Entry(
				"Missing RuntimeKey in DurationFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_DurationFilter{
						DurationFilter: &accessLogService.DurationFilter{
							Comparison: &accessLogService.ComparisonFilter{
								Op: accessLogService.ComparisonFilter_EQ,
								Value: &gloo_envoy_v3.RuntimeUInt32{
									DefaultValue: STATUS_CODE_VALUE,
								},
							},
						},
					},
				},
				NoValueError("DurationFilter", "ComparisonFilter.Value.RuntimeKey"),
			),
			Entry(
				"Bad Subfilter in AndFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_AndFilter{
						AndFilter: &accessLogService.AndFilter{
							Filters: []*accessLogService.AccessLogFilter{
								{
									FilterSpecifier: &accessLogService.AccessLogFilter_HeaderFilter{},
								},
								{
									FilterSpecifier: &accessLogService.AccessLogFilter_ResponseFlagFilter{},
								},
								{
									FilterSpecifier: &accessLogService.AccessLogFilter_RuntimeFilter{
										RuntimeFilter: &accessLogService.RuntimeFilter{
											RuntimeKey: FILTER_RUNTIME_KEY,
											PercentSampled: &gloo_envoy_types.FractionalPercent{
												Numerator:   FRACTIONAL_PERCENT_NUMERATOR,
												Denominator: gloo_envoy_types.FractionalPercent_DenominatorType(INVALID_FRACTIONAL_PERCENT_DENOMINATOR_TYPE),
											},
											UseIndependentRandomness: true,
										},
									},
								},
							},
						},
					},
				},
				WrapInvalidEnumValueError(
					"AndFilter",
					InvalidEnumValueError("RuntimeFilter", "FractionalPercent.Denominator", strconv.FormatUint(uint64(INVALID_FRACTIONAL_PERCENT_DENOMINATOR_TYPE), 10)),
				),
			),
			Entry(
				"Bad Subfilter in OrFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_OrFilter{
						OrFilter: &accessLogService.OrFilter{
							Filters: []*accessLogService.AccessLogFilter{
								{
									FilterSpecifier: &accessLogService.AccessLogFilter_HeaderFilter{},
								},
								{
									FilterSpecifier: &accessLogService.AccessLogFilter_ResponseFlagFilter{},
								},
								{
									FilterSpecifier: &accessLogService.AccessLogFilter_RuntimeFilter{
										RuntimeFilter: &accessLogService.RuntimeFilter{
											RuntimeKey: FILTER_RUNTIME_KEY,
											PercentSampled: &gloo_envoy_types.FractionalPercent{
												Numerator:   FRACTIONAL_PERCENT_NUMERATOR,
												Denominator: gloo_envoy_types.FractionalPercent_DenominatorType(INVALID_FRACTIONAL_PERCENT_DENOMINATOR_TYPE),
											},
											UseIndependentRandomness: true,
										},
									},
								},
							},
						},
					},
				},
				WrapInvalidEnumValueError(
					"OrFilter",
					InvalidEnumValueError("RuntimeFilter", "FractionalPercent.Denominator", strconv.FormatUint(uint64(INVALID_FRACTIONAL_PERCENT_DENOMINATOR_TYPE), 10)),
				),
			),
			Entry(
				"Bad status in GrpcStatusFilter",
				&accessLogService.AccessLogFilter{
					FilterSpecifier: &accessLogService.AccessLogFilter_GrpcStatusFilter{
						GrpcStatusFilter: &accessLogService.GrpcStatusFilter{
							Statuses: []accessLogService.GrpcStatusFilter_Status{100},
							Exclude:  false,
						},
					},
				},
				InvalidEnumValueError("GrpcStatusFilter", "Status", "100"),
			),
		)

	})

	Context("ProcessAccessLogPlugins", func() {

		var alsSettings *accessLogService.AccessLoggingService

		Context("grpc", func() {

			var (
				usRef *core.ResourceRef

				logName      string
				extraHeaders []string
			)

			BeforeEach(func() {
				logName = "test"
				extraHeaders = []string{"test"}
				usRef = &core.ResourceRef{
					Name:      "default",
					Namespace: "default",
				}
				alsSettings = &accessLogService.AccessLoggingService{
					AccessLog: []*accessLogService.AccessLog{
						{
							OutputDestination: &accessLogService.AccessLog_GrpcService{
								GrpcService: &accessLogService.GrpcService{
									LogName: logName,
									ServiceRef: &accessLogService.GrpcService_StaticClusterName{
										StaticClusterName: translatorutil.UpstreamToClusterName(usRef),
									},
									AdditionalRequestHeadersToLog:   extraHeaders,
									AdditionalResponseHeadersToLog:  extraHeaders,
									AdditionalResponseTrailersToLog: extraHeaders,
								},
							},
						},
					},
				}
			})

			It("works", func() {
				accessLogConfigs, err := ProcessAccessLogPlugins(alsSettings, nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(accessLogConfigs).To(HaveLen(1))
				alConfig := accessLogConfigs[0]

				Expect(alConfig.Name).To(Equal(wellknown.HTTPGRPCAccessLog))
				var falCfg envoygrpc.HttpGrpcAccessLogConfig
				err = translatorutil.ParseTypedConfig(alConfig, &falCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(falCfg.AdditionalRequestHeadersToLog).To(Equal(extraHeaders))
				Expect(falCfg.AdditionalResponseHeadersToLog).To(Equal(extraHeaders))
				Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
				Expect(falCfg.CommonConfig.LogName).To(Equal(logName))
				envoyGrpc := falCfg.CommonConfig.GetGrpcService().GetEnvoyGrpc()
				Expect(envoyGrpc).NotTo(BeNil())
				Expect(envoyGrpc.ClusterName).To(Equal(translatorutil.UpstreamToClusterName(usRef)))
			})

		})

		Context("file", func() {

			var (
				strFormat, path string
				jsonFormat      *structpb.Struct
				fsStrFormat     *accessLogService.FileSink_StringFormat
				fsJsonFormat    *accessLogService.FileSink_JsonFormat
			)

			BeforeEach(func() {
				strFormat, path = "formatting string", "path"
				jsonFormat = &structpb.Struct{
					Fields: map[string]*structpb.Value{},
				}
				fsStrFormat = &accessLogService.FileSink_StringFormat{
					StringFormat: strFormat,
				}
				fsJsonFormat = &accessLogService.FileSink_JsonFormat{
					JsonFormat: jsonFormat,
				}
			})

			Context("string", func() {

				BeforeEach(func() {
					alsSettings = &accessLogService.AccessLoggingService{
						AccessLog: []*accessLogService.AccessLog{
							{
								OutputDestination: &accessLogService.AccessLog_FileSink{
									FileSink: &accessLogService.FileSink{
										Path:         path,
										OutputFormat: fsStrFormat,
									},
								},
							},
						},
					}
				})

				It("works", func() {
					accessLogConfigs, err := ProcessAccessLogPlugins(alsSettings, nil)
					Expect(err).NotTo(HaveOccurred())

					Expect(accessLogConfigs).To(HaveLen(1))
					alConfig := accessLogConfigs[0]

					Expect(alConfig.Name).To(Equal(wellknown.FileAccessLog))
					var falCfg envoyalfile.FileAccessLog
					err = translatorutil.ParseTypedConfig(alConfig, &falCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(falCfg.Path).To(Equal(path))
					str := falCfg.GetLogFormat().GetTextFormat()
					Expect(str).To(Equal(strFormat))
				})

			})

			Context("json", func() {

				BeforeEach(func() {
					alsSettings = &accessLogService.AccessLoggingService{
						AccessLog: []*accessLogService.AccessLog{
							{
								OutputDestination: &accessLogService.AccessLog_FileSink{
									FileSink: &accessLogService.FileSink{
										Path:         path,
										OutputFormat: fsJsonFormat,
									},
								},
							},
						},
					}
				})

				It("works", func() {
					accessLogConfigs, err := ProcessAccessLogPlugins(alsSettings, nil)
					Expect(err).NotTo(HaveOccurred())

					Expect(accessLogConfigs).To(HaveLen(1))
					alConfig := accessLogConfigs[0]

					Expect(alConfig.Name).To(Equal(wellknown.FileAccessLog))
					var falCfg envoyalfile.FileAccessLog
					err = translatorutil.ParseTypedConfig(alConfig, &falCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(falCfg.Path).To(Equal(path))
					jsn := falCfg.GetLogFormat().GetJsonFormat()
					Expect(jsn).To(matchers.MatchProto(jsonFormat))
				})

			})
		})

	})

	Context("ProcessHcmNetworkFilter", func() {

		var (
			plugin       plugins.HttpConnectionManagerPlugin
			pluginParams plugins.Params

			parentListener *v1.Listener
			listener       *v1.HttpListener

			envoyHcmConfig *envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager
		)

		BeforeEach(func() {
			plugin = NewPlugin()
			pluginParams = plugins.Params{}

			parentListener = &v1.Listener{}
			listener = &v1.HttpListener{}

			envoyHcmConfig = &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{}
		})

		When("parent listener has no access log settings defined", func() {

			BeforeEach(func() {
				parentListener.Options = nil
			})

			It("does not configure access log config", func() {
				err := plugin.ProcessHcmNetworkFilter(pluginParams, parentListener, listener, envoyHcmConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(envoyHcmConfig.GetAccessLog()).To(BeNil())
			})

		})

		When("parent listener has access log settings defined", func() {

			BeforeEach(func() {
				logName := "test"
				extraHeaders := []string{"test"}
				usRef := &core.ResourceRef{
					Name:      "default",
					Namespace: "default",
				}
				parentListener.Options = &v1.ListenerOptions{
					AccessLoggingService: &accessLogService.AccessLoggingService{
						AccessLog: []*accessLogService.AccessLog{
							{
								OutputDestination: &accessLogService.AccessLog_GrpcService{
									GrpcService: &accessLogService.GrpcService{
										LogName: logName,
										ServiceRef: &accessLogService.GrpcService_StaticClusterName{
											StaticClusterName: translatorutil.UpstreamToClusterName(usRef),
										},
										AdditionalRequestHeadersToLog:   extraHeaders,
										AdditionalResponseHeadersToLog:  extraHeaders,
										AdditionalResponseTrailersToLog: extraHeaders,
									},
								},
							},
						},
					},
				}
			})

			It("does configure access log config", func() {
				err := plugin.ProcessHcmNetworkFilter(pluginParams, parentListener, listener, envoyHcmConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(envoyHcmConfig.GetAccessLog()).NotTo(BeNil())
			})

		})

		When("parent listener has access log settings with filters defined", func() {

			BeforeEach(func() {
				logName := "test"
				extraHeaders := []string{"test"}
				usRef := &core.ResourceRef{
					Name:      "default",
					Namespace: "default",
				}
				filter_runtime_key := "default"
				parentListener.Options = &v1.ListenerOptions{
					AccessLoggingService: &accessLogService.AccessLoggingService{
						AccessLog: []*accessLogService.AccessLog{
							{
								OutputDestination: &accessLogService.AccessLog_GrpcService{
									GrpcService: &accessLogService.GrpcService{
										LogName: logName,
										ServiceRef: &accessLogService.GrpcService_StaticClusterName{
											StaticClusterName: translatorutil.UpstreamToClusterName(usRef),
										},
										AdditionalRequestHeadersToLog:   extraHeaders,
										AdditionalResponseHeadersToLog:  extraHeaders,
										AdditionalResponseTrailersToLog: extraHeaders,
									},
								},

								Filter: &accessLogService.AccessLogFilter{
									FilterSpecifier: &accessLogService.AccessLogFilter_RuntimeFilter{
										RuntimeFilter: &accessLogService.RuntimeFilter{
											RuntimeKey: filter_runtime_key,
											PercentSampled: &gloo_envoy_types.FractionalPercent{
												Numerator:   FRACTIONAL_PERCENT_NUMERATOR,
												Denominator: gloo_envoy_types.FractionalPercent_DenominatorType(FRACTIONAL_PERCENT_DENOMINATOR_TYPE),
											},
											UseIndependentRandomness: true,
										},
									},
								},
							},
						},
					},
				}
			})

			It("does configure access log config", func() {
				err := plugin.ProcessHcmNetworkFilter(pluginParams, parentListener, listener, envoyHcmConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(envoyHcmConfig.GetAccessLog()).NotTo(BeNil())
			})

		})

	})

})
