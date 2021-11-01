package hcm_test

import (
	"time"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	envoy_config_tracing_v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/trace/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	tracingv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tracing"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	mock_hcm "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm/mocks"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	. "github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Plugin", func() {

	var (
		ctrl        *gomock.Controller
		mockTracing *mock_hcm.MockHcmPlugin
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockTracing = mock_hcm.NewMockHcmPlugin(ctrl)
	})

	It("copy all settings to hcm filter", func() {
		collectorUs := v1.NewUpstream("default", "valid")
		snapshot := &v1.ApiSnapshot{
			Upstreams: v1.UpstreamList{collectorUs},
		}

		hcms := &hcm.HttpConnectionManagerSettings{
			UseRemoteAddress:    &wrappers.BoolValue{Value: false},
			XffNumTrustedHops:   5,
			SkipXffAppend:       true,
			Via:                 "Via",
			GenerateRequestId:   &wrappers.BoolValue{Value: false},
			Proxy_100Continue:   true,
			StreamIdleTimeout:   prototime.DurationToProto(time.Hour),
			IdleTimeout:         prototime.DurationToProto(time.Hour),
			MaxRequestHeadersKb: &wrappers.UInt32Value{Value: 5},
			RequestTimeout:      prototime.DurationToProto(time.Hour),
			DrainTimeout:        prototime.DurationToProto(time.Hour),
			DelayedCloseTimeout: prototime.DurationToProto(time.Hour),
			ServerName:          "ServerName",

			AcceptHttp_10:             true,
			ProperCaseHeaderKeyFormat: true,
			DefaultHostForHttp_10:     "DefaultHostForHttp_10",

			Tracing: &tracingv1.ListenerTracingSettings{
				RequestHeadersForTags: []string{"path", "origin"},
				Verbose:               true,
				ProviderConfig: &tracingv1.ListenerTracingSettings_ZipkinConfig{
					ZipkinConfig: &envoy_config_tracing_v3.ZipkinConfig{
						CollectorCluster: &envoy_config_tracing_v3.ZipkinConfig_CollectorUpstreamRef{
							CollectorUpstreamRef: collectorUs.Metadata.Ref(),
						},
						CollectorEndpointVersion: envoy_config_tracing_v3.ZipkinConfig_HTTP_JSON,
						CollectorEndpoint:        "/api/v2/spans",
						SharedSpanContext:        nil,
						TraceId_128Bit:           false,
					},
				},
			},

			ForwardClientCertDetails: hcm.HttpConnectionManagerSettings_APPEND_FORWARD,
			SetCurrentClientCertDetails: &hcm.HttpConnectionManagerSettings_SetCurrentClientCertDetails{
				Subject: &wrappers.BoolValue{Value: true},
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
							Enabled: &wrappers.BoolValue{Value: true},
						},
					},
				},
			},
			MaxConnectionDuration:        prototime.DurationToProto(time.Hour),
			MaxStreamDuration:            prototime.DurationToProto(time.Hour),
			MaxHeadersCount:              &wrappers.UInt32Value{Value: 5},
			CodecType:                    1,
			ServerHeaderTransformation:   hcm.HttpConnectionManagerSettings_OVERWRITE,
			PathWithEscapedSlashesAction: hcm.HttpConnectionManagerSettings_REJECT_REQUEST,
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

		filters := []*envoy_config_listener_v3.Filter{{
			Name: wellknown.HTTPConnectionManager,
		}}

		outl := &envoy_config_listener_v3.Listener{
			FilterChains: []*envoy_config_listener_v3.FilterChain{{
				Filters: filters,
			}},
		}

		p := NewPlugin()
		mockTracing.EXPECT().ProcessHcmSettings(snapshot, gomock.Any(), hcms).Return(nil)
		pluginsList := []plugins.Plugin{mockTracing, p}
		p.RegisterHcmPlugins(pluginsList)
		err := p.ProcessListener(plugins.Params{Snapshot: snapshot}, in, outl)
		Expect(err).NotTo(HaveOccurred())

		var cfg envoyhttp.HttpConnectionManager
		err = translatorutil.ParseTypedConfig(filters[0], &cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.UseRemoteAddress).To(Equal(hcms.UseRemoteAddress))
		Expect(cfg.XffNumTrustedHops).To(Equal(hcms.XffNumTrustedHops))
		Expect(cfg.SkipXffAppend).To(Equal(hcms.SkipXffAppend))
		Expect(cfg.Via).To(Equal(hcms.Via))
		Expect(cfg.GenerateRequestId).To(Equal(hcms.GenerateRequestId))
		Expect(cfg.Proxy_100Continue).To(Equal(hcms.Proxy_100Continue))
		Expect(cfg.StreamIdleTimeout).To(MatchProto(hcms.StreamIdleTimeout))
		Expect(cfg.MaxRequestHeadersKb).To(MatchProto(hcms.MaxRequestHeadersKb))
		Expect(cfg.RequestTimeout).To(MatchProto(hcms.RequestTimeout))
		Expect(cfg.DrainTimeout).To(MatchProto(hcms.DrainTimeout))
		Expect(cfg.DelayedCloseTimeout).To(MatchProto(hcms.DelayedCloseTimeout))
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
		Expect(cfg.CommonHttpProtocolOptions.IdleTimeout).To(MatchProto(hcms.IdleTimeout))
		Expect(cfg.CommonHttpProtocolOptions.GetMaxConnectionDuration()).To(MatchProto(hcms.MaxConnectionDuration))
		Expect(cfg.CommonHttpProtocolOptions.GetMaxStreamDuration()).To(MatchProto(hcms.MaxStreamDuration))
		Expect(cfg.CommonHttpProtocolOptions.GetMaxHeadersCount()).To(MatchProto(hcms.MaxHeadersCount))
		Expect(cfg.GetCodecType()).To(Equal(envoyhttp.HttpConnectionManager_HTTP1))

		Expect(cfg.GetServerHeaderTransformation()).To(Equal(envoyhttp.HttpConnectionManager_OVERWRITE))
		Expect(cfg.GetPathWithEscapedSlashesAction()).To(Equal(envoyhttp.HttpConnectionManager_REJECT_REQUEST))

		// Confirm that MockTracingPlugin return the proper value
		Expect(cfg.Tracing).To(BeNil())

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

	It("copy server_header_transformation setting to hcm filter", func() {
		hcms := &hcm.HttpConnectionManagerSettings{
			ServerHeaderTransformation: hcm.HttpConnectionManagerSettings_PASS_THROUGH,
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

		filters := []*envoy_config_listener_v3.Filter{{
			Name: wellknown.HTTPConnectionManager,
		}}

		outl := &envoy_config_listener_v3.Listener{
			FilterChains: []*envoy_config_listener_v3.FilterChain{{
				Filters: filters,
			}},
		}

		p := NewPlugin()
		mockTracing.EXPECT().ProcessHcmSettings(nil, gomock.Any(), hcms).Return(nil)
		pluginsList := []plugins.Plugin{mockTracing, p}
		p.RegisterHcmPlugins(pluginsList)
		err := p.ProcessListener(plugins.Params{}, in, outl)
		Expect(err).NotTo(HaveOccurred())

		var cfg envoyhttp.HttpConnectionManager
		err = translatorutil.ParseTypedConfig(filters[0], &cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.GetServerHeaderTransformation()).To(Equal(envoyhttp.HttpConnectionManager_PASS_THROUGH))
	})

	Context("upgrades", func() {

		var (
			hcms    *hcm.HttpConnectionManagerSettings
			hl      *v1.HttpListener
			in      *v1.Listener
			outl    *envoy_config_listener_v3.Listener
			filters []*envoy_config_listener_v3.Filter
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

			filters = []*envoy_config_listener_v3.Filter{{
				Name: wellknown.HTTPConnectionManager,
			}}

			outl = &envoy_config_listener_v3.Listener{
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: filters,
				}},
			}

			p = NewPlugin()
		})

		It("enables websockets by default", func() {

			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoyhttp.HttpConnectionManager
			err = translatorutil.ParseTypedConfig(filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(cfg.GetUpgradeConfigs())).To(Equal(1))
			Expect(cfg.GetUpgradeConfigs()[0].UpgradeType).To(Equal("websocket"))
		})

		It("enables websockets by default with no settings", func() {
			hl.Options = nil

			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoyhttp.HttpConnectionManager
			err = translatorutil.ParseTypedConfig(filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(cfg.GetUpgradeConfigs())).To(Equal(1))
			Expect(cfg.GetUpgradeConfigs()[0].UpgradeType).To(Equal("websocket"))
		})

		It("should error when there's a duplicate upgrade config", func() {
			hcms.Upgrades = []*protocol_upgrade.ProtocolUpgradeConfig{
				{
					UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
						Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
							Enabled: &wrappers.BoolValue{Value: true},
						},
					},
				},
				{
					UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
						Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
							Enabled: &wrappers.BoolValue{Value: true},
						},
					},
				},
			}

			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).To(MatchError(ContainSubstring("upgrade config websocket is not unique")))

		})

	})
})
