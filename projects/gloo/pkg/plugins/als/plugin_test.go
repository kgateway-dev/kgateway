package als_test

import (
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
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {
	It("can properly create AccessLog as string", func() {
		strFormat, path := "formatting string", "path"
		fileConfig := &als.AccessLog_FileSink{
			FileSink: &als.FileSink{
				Path: path,
				OutputFormat: &als.FileSink_StringFormat{
					StringFormat: strFormat,
				},
			},
		}

		alsConfig := &als.AccessLoggingService{
			AccessLog: []*als.AccessLog{
				{
					OutputDestination: fileConfig,
				},
			},
		}



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
		Expect(al.Name).To(Equal(envoyutil.FileAccessLog))

		var falCfg envoyalcfg.FileAccessLog
		err = translatorutil.ParseConfig(al, &falCfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(falCfg.Path).To(Equal(path))
	})

	It("can properly create AccessLog as string", func() {
		path := "path"
		jsonFormat := &types.Struct{}
		fileConfig := &als.AccessLog_FileSink{
			FileSink: &als.FileSink{
				Path: path,
				OutputFormat: &als.FileSink_JsonFormat{
					JsonFormat: jsonFormat,
				},
			},
		}

		alsConfig := &als.AccessLoggingService{
			AccessLog: []*als.AccessLog{
				{
					OutputDestination: fileConfig,
				},
			},
		}
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
		Expect(al.Name).To(Equal(envoyutil.FileAccessLog))

		var falCfg envoyalcfg.FileAccessLog
		err = translatorutil.ParseConfig(al, &falCfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(falCfg.Path).To(Equal(path))
	})
})
