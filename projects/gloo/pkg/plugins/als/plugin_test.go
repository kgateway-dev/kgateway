package als_test

import (
	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoygrpc "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {
	var (
		alsConfig *als.AccessLoggingService
	)
	Context("grpc", func() {

		var (
			params plugins.Params
			usRef  core.ResourceRef

			logName      string
			extraHeaders []string
		)

		var checkConfig = func(al *envoyal.AccessLog) {
			Expect(al.Name).To(Equal(wellknown.HTTPGRPCAccessLog))
			var falCfg envoygrpc.HttpGrpcAccessLogConfig
			err := translatorutil.ParseTypedConfig(al, &falCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
			Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
			Expect(falCfg.AdditionalResponseTrailersToLog).To(Equal(extraHeaders))
			Expect(falCfg.CommonConfig.LogName).To(Equal(logName))
			envoyGrpc := falCfg.CommonConfig.GetGrpcService().GetEnvoyGrpc()
			Expect(envoyGrpc).NotTo(BeNil())
			Expect(envoyGrpc.ClusterName).To(Equal(translatorutil.UpstreamToClusterName(usRef)))
		}

		BeforeEach(func() {
			logName = "test"
			extraHeaders = []string{"test"}
			usRef = core.ResourceRef{
				Name:      "default",
				Namespace: "default",
			}
			alsConfig = &als.AccessLoggingService{
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
			params = plugins.Params{
				Snapshot: &v1.ApiSnapshot{
					Upstreams: v1.UpstreamList{
						{
							// UpstreamSpec: nil,
							Metadata: core.Metadata{
								Name:      usRef.Name,
								Namespace: usRef.Namespace,
							},
						},
					},
				},
			}
		})
		It("http", func() {
			hl := &v1.HttpListener{}

			in := &v1.Listener{
				ListenerType: &v1.Listener_HttpListener{
					HttpListener: hl,
				},
				Options: &v1.ListenerOptions{
					AccessLoggingService: alsConfig,
				},
			}

			filters := []*envoylistener.Filter{{
				Name: wellknown.HTTPConnectionManager,
			}}

			outl := &envoyapi.Listener{
				FilterChains: []*envoylistener.FilterChain{{
					Filters: filters,
				}},
			}

			p := NewPlugin()
			err := p.ProcessListener(params, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoyhttp.HttpConnectionManager
			err = translatorutil.ParseTypedConfig(filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.AccessLog).To(HaveLen(1))
			al := cfg.AccessLog[0]
			checkConfig(al)
		})
		It("tcp", func() {
			tl := &v1.TcpListener{}
			in := &v1.Listener{
				ListenerType: &v1.Listener_TcpListener{
					TcpListener: tl,
				},
				Options: &v1.ListenerOptions{
					AccessLoggingService: alsConfig,
				},
			}

			filters := []*envoylistener.Filter{{
				Name: wellknown.TCPProxy,
			}}

			outl := &envoyapi.Listener{
				FilterChains: []*envoylistener.FilterChain{{
					Filters: filters,
				}},
			}

			p := NewPlugin()
			err := p.ProcessListener(params, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.AccessLog).To(HaveLen(1))
			al := cfg.AccessLog[0]
			checkConfig(al)
		})
	})

	Context("file", func() {
		var (
			strFormat, path string
			jsonFormat      *types.Struct
			fsStrFormat     *als.FileSink_StringFormat
			fsJsonFormat    *als.FileSink_JsonFormat
		)

		BeforeEach(func() {
			strFormat, path = "formatting string", "path"
			jsonFormat = &types.Struct{
				Fields: nil,
			}
			fsStrFormat = &als.FileSink_StringFormat{
				StringFormat: strFormat,
			}
			fsJsonFormat = &als.FileSink_JsonFormat{
				JsonFormat: jsonFormat,
			}
		})

		Context("string", func() {

			var checkConfig = func(al *envoyal.AccessLog) {
				Expect(al.Name).To(Equal(wellknown.FileAccessLog))
				var falCfg envoyalfile.FileAccessLog
				err := translatorutil.ParseTypedConfig(al, &falCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(falCfg.Path).To(Equal(path))
				str := falCfg.GetLogFormat().GetTextFormat()
				Expect(str).To(Equal(strFormat))
			}

			BeforeEach(func() {
				alsConfig = &als.AccessLoggingService{
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
			It("http", func() {
				hl := &v1.HttpListener{}

				in := &v1.Listener{
					ListenerType: &v1.Listener_HttpListener{
						HttpListener: hl,
					},
					Options: &v1.ListenerOptions{
						AccessLoggingService: alsConfig,
					},
				}

				filters := []*envoylistener.Filter{{
					Name: wellknown.HTTPConnectionManager,
				}}

				outl := &envoyapi.Listener{
					FilterChains: []*envoylistener.FilterChain{{
						Filters: filters,
					}},
				}

				p := NewPlugin()
				err := p.ProcessListener(plugins.Params{}, in, outl)
				Expect(err).NotTo(HaveOccurred())

				var cfg envoyhttp.HttpConnectionManager
				err = translatorutil.ParseTypedConfig(filters[0], &cfg)
				Expect(err).NotTo(HaveOccurred())

				Expect(cfg.AccessLog).To(HaveLen(1))
				al := cfg.AccessLog[0]
				checkConfig(al)
			})
			It("tcp", func() {
				tl := &v1.TcpListener{}
				in := &v1.Listener{
					ListenerType: &v1.Listener_TcpListener{
						TcpListener: tl,
					},
					Options: &v1.ListenerOptions{
						AccessLoggingService: alsConfig,
					},
				}

				filters := []*envoylistener.Filter{{
					Name: wellknown.TCPProxy,
				}}

				outl := &envoyapi.Listener{
					FilterChains: []*envoylistener.FilterChain{{
						Filters: filters,
					}},
				}

				p := NewPlugin()
				err := p.ProcessListener(plugins.Params{}, in, outl)
				Expect(err).NotTo(HaveOccurred())

				var cfg envoytcp.TcpProxy
				err = translatorutil.ParseTypedConfig(filters[0], &cfg)
				Expect(err).NotTo(HaveOccurred())

				Expect(cfg.AccessLog).To(HaveLen(1))
				al := cfg.AccessLog[0]
				checkConfig(al)
			})

		})

		Context("json", func() {
			var checkConfig = func(al *envoyal.AccessLog) {
				Expect(al.Name).To(Equal(wellknown.FileAccessLog))
				var falCfg envoyalfile.FileAccessLog
				err := translatorutil.ParseTypedConfig(al, &falCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(falCfg.Path).To(Equal(path))
				jsn := falCfg.GetLogFormat().GetJsonFormat()
				Expect(protoutils.StructPbToGogo(jsn)).To(Equal(jsonFormat))
			}

			BeforeEach(func() {
				alsConfig = &als.AccessLoggingService{
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

			It("http", func() {
				hl := &v1.HttpListener{}
				in := &v1.Listener{
					ListenerType: &v1.Listener_HttpListener{
						HttpListener: hl,
					},
					Options: &v1.ListenerOptions{
						AccessLoggingService: alsConfig,
					},
				}

				filters := []*envoylistener.Filter{{
					Name: wellknown.HTTPConnectionManager,
				}}

				outl := &envoyapi.Listener{
					FilterChains: []*envoylistener.FilterChain{{
						Filters: filters,
					}},
				}

				p := NewPlugin()
				err := p.ProcessListener(plugins.Params{}, in, outl)
				Expect(err).NotTo(HaveOccurred())

				var cfg envoyhttp.HttpConnectionManager
				err = translatorutil.ParseTypedConfig(filters[0], &cfg)
				Expect(err).NotTo(HaveOccurred())

				Expect(cfg.AccessLog).To(HaveLen(1))
				al := cfg.AccessLog[0]
				checkConfig(al)
			})
			It("tcp", func() {
				tl := &v1.TcpListener{}
				in := &v1.Listener{
					ListenerType: &v1.Listener_TcpListener{
						TcpListener: tl,
					},
					Options: &v1.ListenerOptions{
						AccessLoggingService: alsConfig,
					},
				}

				filters := []*envoylistener.Filter{{
					Name: wellknown.TCPProxy,
				}}

				outl := &envoyapi.Listener{
					FilterChains: []*envoylistener.FilterChain{{
						Filters: filters,
					}},
				}

				p := NewPlugin()
				err := p.ProcessListener(plugins.Params{}, in, outl)
				Expect(err).NotTo(HaveOccurred())

				var cfg envoytcp.TcpProxy
				err = translatorutil.ParseTypedConfig(filters[0], &cfg)
				Expect(err).NotTo(HaveOccurred())

				Expect(cfg.AccessLog).To(HaveLen(1))
				al := cfg.AccessLog[0]
				checkConfig(al)
			})
		})
	})
})
