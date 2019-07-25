package als_test

import (
	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/als"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyalcfg "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {

	var (
		strFormat, path string
		jsonFormat      *types.Struct
		fsStrFormat     *als.FileSink_StringFormat
		fsJsonFormat    *als.FileSink_JsonFormat
		alsConfig       *als.AccessLoggingService
	)

	BeforeEach(func() {
		strFormat, path = "formatting string", "path"
		jsonFormat = &types.Struct{
			Fields: map[string]*types.Value{},
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
			Expect(al.Name).To(Equal(envoyutil.FileAccessLog))
			var falCfg envoyalcfg.FileAccessLog
			err := translatorutil.ParseConfig(al, &falCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(falCfg.Path).To(Equal(path))
			str := falCfg.GetFormat()
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
				Plugins: &v1.ListenerPlugins{
					Als: alsConfig,
				},
			}

			filters := []envoylistener.Filter{{
				Name: envoyutil.HTTPConnectionManager,
			}}

			outl := &envoyapi.Listener{
				FilterChains: []envoylistener.FilterChain{{
					Filters: filters,
				}},
			}

			p := NewPlugin()
			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoyhttp.HttpConnectionManager
			err = translatorutil.ParseConfig(&filters[0], &cfg)
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
				Plugins: &v1.ListenerPlugins{
					Als: alsConfig,
				},
			}

			filters := []envoylistener.Filter{{
				Name: envoyutil.TCPProxy,
			}}

			outl := &envoyapi.Listener{
				FilterChains: []envoylistener.FilterChain{{
					Filters: filters,
				}},
			}

			p := NewPlugin()
			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseConfig(&filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.AccessLog).To(HaveLen(1))
			al := cfg.AccessLog[0]
			checkConfig(al)
		})

	})

	Context("json", func() {
		var checkConfig = func(al *envoyal.AccessLog) {
			Expect(al.Name).To(Equal(envoyutil.FileAccessLog))
			var falCfg envoyalcfg.FileAccessLog
			err := translatorutil.ParseConfig(al, &falCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(falCfg.Path).To(Equal(path))
			jsn := falCfg.GetJsonFormat()
			Expect(jsn).To(Equal(jsonFormat))
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
				Plugins: &v1.ListenerPlugins{
					Als: alsConfig,
				},
			}

			filters := []envoylistener.Filter{{
				Name: envoyutil.HTTPConnectionManager,
			}}

			outl := &envoyapi.Listener{
				FilterChains: []envoylistener.FilterChain{{
					Filters: filters,
				}},
			}

			p := NewPlugin()
			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoyhttp.HttpConnectionManager
			err = translatorutil.ParseConfig(&filters[0], &cfg)
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
				Plugins: &v1.ListenerPlugins{
					Als: alsConfig,
				},
			}

			filters := []envoylistener.Filter{{
				Name: envoyutil.TCPProxy,
			}}

			outl := &envoyapi.Listener{
				FilterChains: []envoylistener.FilterChain{{
					Filters: filters,
				}},
			}

			p := NewPlugin()
			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseConfig(&filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.AccessLog).To(HaveLen(1))
			al := cfg.AccessLog[0]
			checkConfig(al)
		})
	})
})
