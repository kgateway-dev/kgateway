package hcm_test

import (
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tracing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	tracingv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tracing"
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

			Tracing: &tracingv1.ListenerTracingSettings{
				RequestHeadersForTags: []string{"path", "origin"},
				Verbose:               true,
			},

			ForwardClientCertDetails: hcm.HttpConnectionManagerSettings_APPEND_FORWARD,
			SetCurrentClientCertDetails: &hcm.HttpConnectionManagerSettings_SetCurrentClientCertDetails{
				Subject: &types.BoolValue{Value: true},
				Cert:    true,
				Chain:   true,
				Dns:     true,
				Uri:     true,
			},
		}
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
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
		pluginsList := []plugins.Plugin{tracing.NewPlugin(), p}
		p.RegisterHcmPlugins(pluginsList)
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
		Expect(trace.RandomSampling.Value).To(Equal(100.0))
		Expect(trace.OverallSampling.Value).To(Equal(100.0))

		Expect(cfg.ForwardClientCertDetails).To(Equal(envoyhttp.APPEND_FORWARD))

		ccd := cfg.SetCurrentClientCertDetails
		Expect(ccd.Subject.Value).To(BeTrue())
		Expect(ccd.Cert).To(BeTrue())
		Expect(ccd.Chain).To(BeTrue())
		Expect(ccd.Dns).To(BeTrue())
		Expect(ccd.Uri).To(BeTrue())
	})

})
