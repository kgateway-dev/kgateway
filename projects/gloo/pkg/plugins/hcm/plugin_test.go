package hcm_test

import (
	"time"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/golang/mock/gomock"
	envoy_config_tracing_v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/trace/v3"
	mock_hcm "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm/mocks"

	"github.com/solo-io/gloo/pkg/utils"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	tracingv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tracing"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
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
		pd := func(t time.Duration) *time.Duration { return &t }
		collectorUs := v1.NewUpstream("default", "valid")
		snapshot := &v1.ApiSnapshot{
			Upstreams: v1.UpstreamList{collectorUs},
		}

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
				ProviderConfig: &tracingv1.ListenerTracingSettings_ZipkinConfig{
					ZipkinConfig: &envoy_config_tracing_v3.ZipkinConfig{
						CollectorCluster: &envoy_config_tracing_v3.ZipkinConfig_CollectorUpstreamRef{
							CollectorUpstreamRef: utils.ResourceRefPtr(collectorUs.Metadata.Ref()),
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
			MaxConnectionDuration:      pd(time.Hour),
			MaxStreamDuration:          pd(time.Hour),
			ServerHeaderTransformation: hcm.HttpConnectionManagerSettings_OVERWRITE,
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
		Expect(cfg.CommonHttpProtocolOptions.GetMaxConnectionDuration()).To(Equal(gogoutils.DurationStdToProto(hcms.MaxConnectionDuration)))
		Expect(cfg.CommonHttpProtocolOptions.GetMaxStreamDuration()).To(Equal(gogoutils.DurationStdToProto(hcms.MaxStreamDuration)))
		Expect(cfg.GetServerHeaderTransformation()).To(Equal(envoyhttp.HttpConnectionManager_OVERWRITE))

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
