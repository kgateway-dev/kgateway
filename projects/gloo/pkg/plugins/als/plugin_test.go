package als_test

import (
	"fmt"

	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v31 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
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

	// Because we are just translatating the filters using marshaling/unmarshaling, we should test each filter type
	// to make sure we copied/pasted correctly and that no changes made to the Envoy definitions broke anything
	Describe("Table driven tests to test each filter", func() {
		DescribeTable("Test each filter is translated properly",
			func(glooInputFilter *accessLogService.AccessLogFilter, expectedEnvoyFilter *envoyal.AccessLogFilter) {
				logName := "test"
				extraHeaders := []string{"test"}
				usRef := &core.ResourceRef{
					Name:      "default",
					Namespace: "default",
				}
				alsSettings := &accessLogService.AccessLoggingService{
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
							Filter: glooInputFilter,
						},
					},
				}

				accessLogConfigs, err := ProcessAccessLogPlugins(alsSettings, nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(accessLogConfigs).To(HaveLen(1))
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

				accessLogFilter := accessLogConfig.GetFilter()
				Expect(accessLogFilter).To(matchers.MatchProto(expectedEnvoyFilter))
			},
			Entry(
				"nil filter",
				&accessLogService.AccessLogFilter{},
				&envoyal.AccessLogFilter{}),
			Entry(
				"StatusCodeFilter",
				&accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_StatusCodeFilter{}},
				&envoyal.AccessLogFilter{
					FilterSpecifier: &envoyal.AccessLogFilter_StatusCodeFilter{
						StatusCodeFilter: &envoyal.StatusCodeFilter{},
					},
				}),
			Entry(
				"DurationFilter",
				&accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_DurationFilter{}},
				&envoyal.AccessLogFilter{
					FilterSpecifier: &envoyal.AccessLogFilter_DurationFilter{
						DurationFilter: &envoyal.DurationFilter{},
					},
				}),
			Entry(
				"NotHealthCheckFilter",
				&accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_NotHealthCheckFilter{}},
				&envoyal.AccessLogFilter{
					FilterSpecifier: &envoyal.AccessLogFilter_NotHealthCheckFilter{
						NotHealthCheckFilter: &envoyal.NotHealthCheckFilter{},
					},
				}),
			Entry(
				"TraceableFilter",
				&accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_TraceableFilter{}},
				&envoyal.AccessLogFilter{
					FilterSpecifier: &envoyal.AccessLogFilter_TraceableFilter{
						TraceableFilter: &envoyal.TraceableFilter{},
					},
				}),
			Entry(
				"RuntimeFilter",
				&accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_RuntimeFilter{}},
				&envoyal.AccessLogFilter{
					FilterSpecifier: &envoyal.AccessLogFilter_RuntimeFilter{
						RuntimeFilter: &envoyal.RuntimeFilter{},
					},
				}),
			Entry(
				"AndFilter",
				&accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_AndFilter{}},
				&envoyal.AccessLogFilter{
					FilterSpecifier: &envoyal.AccessLogFilter_AndFilter{
						AndFilter: &envoyal.AndFilter{},
					},
				}),
			Entry(
				"OrFilter",
				&accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_OrFilter{}},
				&envoyal.AccessLogFilter{
					FilterSpecifier: &envoyal.AccessLogFilter_OrFilter{
						OrFilter: &envoyal.OrFilter{},
					},
				}),
			Entry(
				"HeaderFilter",
				&accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_HeaderFilter{}},
				&envoyal.AccessLogFilter{
					FilterSpecifier: &envoyal.AccessLogFilter_HeaderFilter{
						HeaderFilter: &envoyal.HeaderFilter{},
					},
				}),
			Entry(
				"ResponseFlagFilter",
				&accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_ResponseFlagFilter{}},
				&envoyal.AccessLogFilter{
					FilterSpecifier: &envoyal.AccessLogFilter_ResponseFlagFilter{
						ResponseFlagFilter: &envoyal.ResponseFlagFilter{},
					},
				}),
			Entry(
				"GrpcStatusFilter",
				&accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_GrpcStatusFilter{}},
				&envoyal.AccessLogFilter{
					FilterSpecifier: &envoyal.AccessLogFilter_GrpcStatusFilter{
						GrpcStatusFilter: &envoyal.GrpcStatusFilter{},
					},
				}),
		)
	})

	Context("ProcessAccessLogPlugins", func() {

		var (
			alsSettings  *accessLogService.AccessLoggingService
			alsAndFilter *accessLogService.AccessLogFilter_AndFilter
		)

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
							//Filter: &accessLogService.AccessLogFilter{FilterSpecifier: &accessLogService.AccessLogFilter_TraceableFilter{}},
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

			It("Filter test", func() {

				accessLogConfigs, err := ProcessAccessLogPlugins(alsSettings, nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(accessLogConfigs).To(HaveLen(1))
				alConfig := accessLogConfigs[0]

				alsFilter := alConfig.GetFilter().GetFilterSpecifier()
				expectedFilter := &envoyal.AccessLogFilter_TraceableFilter{
					TraceableFilter: &envoyal.TraceableFilter{},
				}

				Expect(alsFilter).To(Equal(expectedFilter))

				fmt.Printf("%+v", alsFilter)
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

		Context("Access log with single filter", func() {

			var (
				usRef *core.ResourceRef

				logName            string
				extraHeaders       []string
				filter_runtime_key string
			)

			BeforeEach(func() {
				logName = "default"
				extraHeaders = []string{"test"}
				usRef = &core.ResourceRef{
					Name:      "default",
					Namespace: "default",
				}
				filter_runtime_key = "10"
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
							Filter: &accessLogService.AccessLogFilter{
								FilterSpecifier: &accessLogService.AccessLogFilter_RuntimeFilter{
									RuntimeFilter: &accessLogService.RuntimeFilter{
										RuntimeKey: filter_runtime_key,
										PercentSampled: &v3.FractionalPercent{
											Numerator:   50,
											Denominator: v3.FractionalPercent_DenominatorType(40),
										},
										UseIndependentRandomness: true,
									},
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
				Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
				Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
				Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
				Expect(falCfg.CommonConfig.LogName).To(Equal(logName))
				envoyGrpc := falCfg.CommonConfig.GetGrpcService().GetEnvoyGrpc()
				Expect(envoyGrpc).NotTo(BeNil())
				Expect(envoyGrpc.ClusterName).To(Equal(translatorutil.UpstreamToClusterName(usRef)))
			})

		})

		Context("Access log with multiple filters", func() {

			var (
				usRef *core.ResourceRef

				logName      string
				extraHeaders []string
			)

			BeforeEach(func() {
				logName = "default"
				extraHeaders = []string{"test"}
				usRef = &core.ResourceRef{
					Name:      "default",
					Namespace: "default",
				}
				alsOrFilter := &accessLogService.OrFilter{
					Filters: []*accessLogService.AccessLogFilter{
						{
							FilterSpecifier: &accessLogService.AccessLogFilter_DurationFilter{
								DurationFilter: &accessLogService.DurationFilter{
									Comparison: &accessLogService.ComparisonFilter{
										Op: accessLogService.ComparisonFilter_EQ,
										Value: &v31.RuntimeUInt32{
											DefaultValue: 2000,
											RuntimeKey:   "access_log.access_error.duration",
										},
									},
								},
							},
						},
						{
							FilterSpecifier: &accessLogService.AccessLogFilter_GrpcStatusFilter{
								GrpcStatusFilter: &accessLogService.GrpcStatusFilter{
									Statuses: []accessLogService.GrpcStatusFilter_Status(accessLogService.GrpcStatusFilter_CANCELED.String()),
								},
							},
						},
					},
				}
				alsAndFilter = &accessLogService.AccessLogFilter_AndFilter{
					AndFilter: &accessLogService.AndFilter{
						Filters: []*accessLogService.AccessLogFilter{
							{
								FilterSpecifier: &accessLogService.AccessLogFilter_RuntimeFilter{
									RuntimeFilter: &accessLogService.RuntimeFilter{
										RuntimeKey:               "filter_runtime_key",
										UseIndependentRandomness: true,
									},
								},
							},
							{
								FilterSpecifier: &accessLogService.AccessLogFilter_StatusCodeFilter{},
							},
							{
								FilterSpecifier: &accessLogService.AccessLogFilter_OrFilter{
									OrFilter: alsOrFilter,
								},
							},
						},
					},
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
							Filter: &accessLogService.AccessLogFilter{
								FilterSpecifier: alsAndFilter,
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
				Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
				Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
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
											PercentSampled: &v3.FractionalPercent{
												Numerator:   50,
												Denominator: v3.FractionalPercent_DenominatorType(40),
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
