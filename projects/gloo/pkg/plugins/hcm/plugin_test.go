package hcm_test

import (
	envoytracing "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	"time"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tracing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
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

			AcceptHttp_10:             true,
			ProperCaseHeaderKeyFormat: true,
			DefaultHostForHttp_10:     "DefaultHostForHttp_10",

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
			PreserveExternalRequestId: true,

			Upgrades: []*protocol_upgrade.ProtocolUpgradeConfig{
				{
					UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
						Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
							Enabled: &types.BoolValue{Value: true},
						},
					},
				},
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

		filters := []*envoylistener.Filter{{
			Name: wellknown.HTTPConnectionManager,
		}}

		outl := &envoyapi.Listener{
			FilterChains: []*envoylistener.FilterChain{{
				Filters: filters,
			}},
		}

		p := NewPlugin()
		pluginsList := []plugins.Plugin{tracing.NewPlugin(), p}
		p.RegisterHcmPlugins(pluginsList)
		err := p.ProcessListener(plugins.Params{}, in, outl)
		Expect(err).NotTo(HaveOccurred())

		var cfg envoyhttp.HttpConnectionManager
		err = translatorutil.ParseConfig(filters[0], &cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.UseRemoteAddress).To(Equal(gogoutils.BoolGogoToProto(hcms.UseRemoteAddress)))
		Expect(cfg.XffNumTrustedHops).To(Equal(hcms.XffNumTrustedHops))
		Expect(cfg.SkipXffAppend).To(Equal(hcms.SkipXffAppend))
		Expect(cfg.Via).To(Equal(hcms.Via))
		Expect(cfg.GenerateRequestId).To(Equal(gogoutils.BoolGogoToProto(hcms.GenerateRequestId)))
		Expect(cfg.Proxy_100Continue).To(Equal(hcms.Proxy_100Continue))
		Expect(cfg.StreamIdleTimeout).To(Equal(gogoutils.DurationStdToProto(hcms.StreamIdleTimeout)))
		Expect(cfg.MaxRequestHeadersKb).To(Equal(gogoutils.UInt32GogoToProto(hcms.MaxRequestHeadersKb)))
		Expect(cfg.RequestTimeout).To(Equal(gogoutils.DurationStdToProto(hcms.RequestTimeout)))
		Expect(cfg.DrainTimeout).To(Equal(gogoutils.DurationStdToProto(hcms.DrainTimeout)))
		Expect(cfg.DelayedCloseTimeout).To(Equal(gogoutils.DurationStdToProto(hcms.DelayedCloseTimeout)))
		Expect(cfg.ServerName).To(Equal(hcms.ServerName))
		Expect(cfg.HttpProtocolOptions.AcceptHttp_10).To(Equal(hcms.AcceptHttp_10))
		if hcms.ProperCaseHeaderKeyFormat {
			Expect(cfg.HttpProtocolOptions.HeaderKeyFormat).To(Equal(&envoycore.Http1ProtocolOptions_HeaderKeyFormat{
				HeaderFormat: &envoycore.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords_{
					ProperCaseWords: &envoycore.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords{},
				},
			}))
		}
		Expect(cfg.HttpProtocolOptions.DefaultHostForHttp_10).To(Equal(hcms.DefaultHostForHttp_10))
		Expect(cfg.PreserveExternalRequestId).To(Equal(hcms.PreserveExternalRequestId))

		Expect(cfg.CommonHttpProtocolOptions).NotTo(BeNil())
		Expect(cfg.CommonHttpProtocolOptions.IdleTimeout).To(Equal(gogoutils.DurationStdToProto(hcms.IdleTimeout)))

		trace := cfg.Tracing
		Expect(trace.CustomTags).To(ConsistOf([]*envoytracing.CustomTag{
			{
				Tag: "path",
				Type: &envoytracing.CustomTag_RequestHeader{
					RequestHeader: &envoytracing.CustomTag_Header{
						Name: "path",
					},
				},
			},
			{
				Tag: "origin",
				Type: &envoytracing.CustomTag_RequestHeader{
					RequestHeader: &envoytracing.CustomTag_Header{
						Name: "origin",
					},
				},
			}}))
		Expect(trace.Verbose).To(BeTrue())
		Expect(trace.ClientSampling.Value).To(Equal(100.0))
		Expect(trace.RandomSampling.Value).To(Equal(100.0))
		Expect(trace.OverallSampling.Value).To(Equal(100.0))

		Expect(len(cfg.UpgradeConfigs)).To(Equal(1))
		Expect(cfg.UpgradeConfigs[0].UpgradeType).To(Equal("websocket"))
		Expect(cfg.UpgradeConfigs[0].Enabled.GetValue()).To(Equal(true))

		Expect(cfg.ForwardClientCertDetails).To(Equal(envoyhttp.HttpConnectionManager_APPEND_FORWARD))

		ccd := cfg.SetCurrentClientCertDetails
		Expect(ccd.Subject.Value).To(BeTrue())
		Expect(ccd.Cert).To(BeTrue())
		Expect(ccd.Chain).To(BeTrue())
		Expect(ccd.Dns).To(BeTrue())
		Expect(ccd.Uri).To(BeTrue())
	})

	Context("upgrades", func() {

		var (
			hcms    *hcm.HttpConnectionManagerSettings
			hl      *v1.HttpListener
			in      *v1.Listener
			outl    *envoyapi.Listener
			filters []*envoylistener.Filter
			p       *Plugin
		)

		BeforeEach(func() {
			hcms = &hcm.HttpConnectionManagerSettings{}

			hl = &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					HttpConnectionManagerSettings: hcms,
				},
			}

			in = &v1.Listener{
				ListenerType: &v1.Listener_HttpListener{
					HttpListener: hl,
				},
			}

			filters = []*envoylistener.Filter{{
				Name: wellknown.HTTPConnectionManager,
			}}

			outl = &envoyapi.Listener{
				FilterChains: []*envoylistener.FilterChain{{
					Filters: filters,
				}},
			}

			p = NewPlugin()
		})

		It("enables websockets by default", func() {

			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoyhttp.HttpConnectionManager
			err = translatorutil.ParseConfig(filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(cfg.GetUpgradeConfigs())).To(Equal(1))
			Expect(cfg.GetUpgradeConfigs()[0].UpgradeType).To(Equal("websocket"))
		})

		It("enables websockets by default with no settings", func() {
			hl.Options = nil

			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoyhttp.HttpConnectionManager
			err = translatorutil.ParseConfig(filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(cfg.GetUpgradeConfigs())).To(Equal(1))
			Expect(cfg.GetUpgradeConfigs()[0].UpgradeType).To(Equal("websocket"))
		})

		It("should error when there's a duplicate upgrade config", func() {
			hcms.Upgrades = []*protocol_upgrade.ProtocolUpgradeConfig{
				{
					UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
						Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
							Enabled: &types.BoolValue{Value: true},
						},
					},
				},
				{
					UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
						Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
							Enabled: &types.BoolValue{Value: true},
						},
					},
				},
			}

			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).To(MatchError(ContainSubstring("upgrade config websocket is not unique")))

		})

	})
})
