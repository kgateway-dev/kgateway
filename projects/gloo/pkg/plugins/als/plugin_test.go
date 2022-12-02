package als_test

import (
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/test/matchers"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoygrpc "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
)

var _ = Describe("Plugin", func() {

	Context("ProcessAccessLogPlugins", func() {

		var (
			alsSettings *als.AccessLoggingService
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
				alsSettings = &als.AccessLoggingService{
					AccessLog: []*als.AccessLog{
						{
							OutputDestination: &als.AccessLog_GrpcService{
								GrpcService: &als.GrpcService{
									LogName: logName,
									ServiceRef: &als.GrpcService_StaticClusterName{
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
				Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
				Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
				Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
				Expect(falCfg.CommonConfig.LogName).To(Equal(logName))
				envoyGrpc := falCfg.CommonConfig.GetGrpcService().GetEnvoyGrpc()
				Expect(envoyGrpc).NotTo(BeNil())
				Expect(envoyGrpc.ClusterName).To(Equal(translatorutil.UpstreamToClusterName(usRef)))
			})

		})

		Context("grpc with filter", func() {

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
				alsSettings = &als.AccessLoggingService{
					AccessLog: []*als.AccessLog{
						{
							OutputDestination: &als.AccessLog_GrpcService{
								GrpcService: &als.GrpcService{
									LogName: logName,
									ServiceRef: &als.GrpcService_StaticClusterName{
										StaticClusterName: translatorutil.UpstreamToClusterName(usRef),
									},
									AdditionalRequestHeadersToLog:   extraHeaders,
									AdditionalResponseHeadersToLog:  extraHeaders,
									AdditionalResponseTrailersToLog: extraHeaders,
									Filter: &als.AccessLogFilter{
										FilterSpecifier: &als.AccessLogFilter_RuntimeFilter{
											RuntimeFilter: &als.RuntimeFilter{
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
				strFormat, path    string
				jsonFormat         *structpb.Struct
				fsStrFormat        *als.FileSink_StringFormat
				fsJsonFormat       *als.FileSink_JsonFormat
				filter_runtime_key string
			)

			BeforeEach(func() {
				strFormat, path = "formatting string", "path"
				jsonFormat = &structpb.Struct{
					Fields: map[string]*structpb.Value{},
				}
				fsStrFormat = &als.FileSink_StringFormat{
					StringFormat: strFormat,
				}
				fsJsonFormat = &als.FileSink_JsonFormat{
					JsonFormat: jsonFormat,
				}
			})

			Context("string", func() {

				BeforeEach(func() {
					alsSettings = &als.AccessLoggingService{
						AccessLog: []*als.AccessLog{
							{
								OutputDestination: &als.AccessLog_FileSink{
									FileSink: &als.FileSink{
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
					alsSettings = &als.AccessLoggingService{
						AccessLog: []*als.AccessLog{
							{
								OutputDestination: &als.AccessLog_FileSink{
									FileSink: &als.FileSink{
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

			Context("with filters", func() {

				BeforeEach(func() {
					alsSettings = &als.AccessLoggingService{
						AccessLog: []*als.AccessLog{
							{
								OutputDestination: &als.AccessLog_FileSink{
									FileSink: &als.FileSink{
										Path:         path,
										OutputFormat: fsJsonFormat,
										Filter: &als.AccessLogFilter{
											FilterSpecifier: &als.AccessLogFilter_RuntimeFilter{
												RuntimeFilter: &als.RuntimeFilter{
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
					AccessLoggingService: &als.AccessLoggingService{
						AccessLog: []*als.AccessLog{
							{
								OutputDestination: &als.AccessLog_GrpcService{
									GrpcService: &als.GrpcService{
										LogName: logName,
										ServiceRef: &als.GrpcService_StaticClusterName{
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
					AccessLoggingService: &als.AccessLoggingService{
						AccessLog: []*als.AccessLog{
							{
								OutputDestination: &als.AccessLog_GrpcService{
									GrpcService: &als.GrpcService{
										LogName: logName,
										ServiceRef: &als.GrpcService_StaticClusterName{
											StaticClusterName: translatorutil.UpstreamToClusterName(usRef),
										},
										AdditionalRequestHeadersToLog:   extraHeaders,
										AdditionalResponseHeadersToLog:  extraHeaders,
										AdditionalResponseTrailersToLog: extraHeaders,
										Filter: &als.AccessLogFilter{
											FilterSpecifier: &als.AccessLogFilter_RuntimeFilter{
												RuntimeFilter: &als.RuntimeFilter{
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
