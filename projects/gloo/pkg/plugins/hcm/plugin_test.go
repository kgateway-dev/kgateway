package hcm_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {
	It("copy all settings to hcm filter", func() {
		pd := func(t time.Duration) *time.Duration { return &t }
		hcms := &hcm.HttpConnectionManagerSettings{
			UseRemoteAddress:    &types.BoolValue{Value: false},
			XffNumTrustedHops:   5,
			SkipXffAppend:       true,
			Via:                 "Via",
			GenerateRequestId:   &types.BoolValue{Value: false},
			Proxy_100Continue:   true,
			StreamIdleTimeout:   pd(time.Hour),
			IdleTimeout:         pd(time.Hour),
			MaxRequestHeadersKb: &types.UInt32Value{Value: 5},
			RequestTimeout:      pd(time.Hour),
			DrainTimeout:        pd(time.Hour),
			DelayedCloseTimeout: pd(time.Hour),
			ServerName:          "ServerName",

			AcceptHttp_10:         true,
			DefaultHostForHttp_10: "DefaultHostForHttp_10",

			Tracing: &hcm.HttpConnectionManagerSettings_TracingSettings{
				RequestHeadersForTags: []string{"path", "origin"},
				Verbose:               true,
			},
		}
		hl := &v1.HttpListener{
			Plugins: &v1.HttpListenerPlugins{
				HttpConnectionManagerSettings: hcms,
			},
		}

		in := &v1.Listener{
			ListenerType: &v1.Listener_HttpListener{
				HttpListener: hl,
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

		Expect(cfg.UseRemoteAddress).To(Equal(hcms.UseRemoteAddress))
		Expect(cfg.XffNumTrustedHops).To(Equal(hcms.XffNumTrustedHops))
		Expect(cfg.SkipXffAppend).To(Equal(hcms.SkipXffAppend))
		Expect(cfg.Via).To(Equal(hcms.Via))
		Expect(cfg.GenerateRequestId).To(Equal(hcms.GenerateRequestId))
		Expect(cfg.Proxy_100Continue).To(Equal(hcms.Proxy_100Continue))
		Expect(cfg.StreamIdleTimeout).To(Equal(hcms.StreamIdleTimeout))
		Expect(cfg.IdleTimeout).To(Equal(hcms.IdleTimeout))
		Expect(cfg.MaxRequestHeadersKb).To(Equal(hcms.MaxRequestHeadersKb))
		Expect(cfg.RequestTimeout).To(Equal(hcms.RequestTimeout))
		Expect(cfg.DrainTimeout).To(Equal(hcms.DrainTimeout))
		Expect(cfg.DelayedCloseTimeout).To(Equal(hcms.DelayedCloseTimeout))
		Expect(cfg.ServerName).To(Equal(hcms.ServerName))
		Expect(cfg.HttpProtocolOptions.AcceptHttp_10).To(Equal(hcms.AcceptHttp_10))
		Expect(cfg.HttpProtocolOptions.DefaultHostForHttp_10).To(Equal(hcms.DefaultHostForHttp_10))

		trace := cfg.Tracing
		Expect(trace.RequestHeadersForTags).To(ConsistOf([]string{"path", "origin"}))
		Expect(trace.Verbose).To(BeTrue())
		Expect(trace.ClientSampling.Value).To(Equal(100.0))
		Expect(trace.RandomSampling.Value).To(Equal(0.0))
		Expect(trace.OverallSampling.Value).To(Equal(100.0))
	})

	It("should update routes properly", func() {
		p := NewPlugin()
		in := &v1.Route{}
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).NotTo(HaveOccurred())

		inFull := &v1.Route{
			Matcher: nil,
			Action:  nil,
			RoutePlugins: &v1.RoutePlugins{
				Tracing: &hcm.RouteTracingSettings{
					RouteDescriptor: "hello",
				},
			},
		}
		outFull := &envoyroute.Route{}
		err = p.ProcessRoute(plugins.RouteParams{}, inFull, outFull)
		Expect(err).NotTo(HaveOccurred())
		Expect(outFull.Decorator.Operation).To(Equal("hello"))
		Expect(outFull.Tracing.ClientSampling.Numerator).To(Equal(uint32(100)))
		Expect(outFull.Tracing.RandomSampling.Numerator).To(Equal(uint32(0)))
		Expect(outFull.Tracing.OverallSampling.Numerator).To(Equal(uint32(100)))
	})

})
