package hcm

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/util"
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.ListenerPlugin = new(Plugin)

type Plugin struct {
	hcmPlugins []HcmPlugin
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) RegisterHcmPlugins(allPlugins []plugins.Plugin) {
	for _, plugin := range allPlugins {
		if hp, ok := plugin.(HcmPlugin); ok {
			p.hcmPlugins = append(p.hcmPlugins, hp)
		}
	}
}

// ProcessListener has two responsibilities:
// 1. apply the core HCM settings from the HCM plugin to the listener
// 2. call each of the HCM plugins to make sure that they have a chance to apply their modifications to the listener
func (p *Plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoyapi.Listener) error {
	hl, ok := in.ListenerType.(*v1.Listener_HttpListener)
	if !ok {
		return nil
	}
	if hl.HttpListener == nil {
		return nil
	}
	var hcmSettings *hcm.HttpConnectionManagerSettings
	if hl.HttpListener.GetOptions() != nil {
		hcmSettings = hl.HttpListener.GetOptions().HttpConnectionManagerSettings
	}
	if hcmSettings == nil && len(p.hcmPlugins) == 0 {
		// special case where we have nothing to do
		return nil
	}

	for _, f := range out.FilterChains {
		for i, filter := range f.Filters {
			if filter.Name == util.HTTPConnectionManager {
				// get config
				var cfg envoyhttp.HttpConnectionManager
				err := translatorutil.ParseConfig(filter, &cfg)
				// this should never error
				if err != nil {
					return err
				}

				// first apply the core HCM settings, if any
				if hcmSettings != nil {
					copyCoreHcmSettings(&cfg, hcmSettings)
				}

				// then allow any HCM plugins to make their changes, with respect to any changes the core plugin made
				for _, hp := range p.hcmPlugins {
					if err := hp.ProcessHcmSettings(&cfg, hcmSettings); err != nil {
						return hcmPluginError(err)
					}
				}

				f.Filters[i], err = translatorutil.NewFilterWithConfig(util.HTTPConnectionManager, &cfg)
				// this should never error
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func copyCoreHcmSettings(cfg *envoyhttp.HttpConnectionManager, hcmSettings *hcm.HttpConnectionManagerSettings) {
	cfg.UseRemoteAddress = gogoutils.BoolGogoToProto(hcmSettings.UseRemoteAddress)
	cfg.XffNumTrustedHops = hcmSettings.XffNumTrustedHops
	cfg.SkipXffAppend = hcmSettings.SkipXffAppend
	cfg.Via = hcmSettings.Via
	cfg.GenerateRequestId = gogoutils.BoolGogoToProto(hcmSettings.GenerateRequestId)
	cfg.Proxy_100Continue = hcmSettings.Proxy_100Continue
	cfg.StreamIdleTimeout = gogoutils.DurationStdToProto(hcmSettings.StreamIdleTimeout)
	cfg.IdleTimeout = gogoutils.DurationStdToProto(hcmSettings.IdleTimeout)
	cfg.MaxRequestHeadersKb = gogoutils.UInt32GogoToProto(hcmSettings.MaxRequestHeadersKb)
	cfg.RequestTimeout = gogoutils.DurationStdToProto(hcmSettings.RequestTimeout)
	cfg.DrainTimeout = gogoutils.DurationStdToProto(hcmSettings.DrainTimeout)
	cfg.DelayedCloseTimeout = gogoutils.DurationStdToProto(hcmSettings.DelayedCloseTimeout)
	cfg.ServerName = hcmSettings.ServerName
	cfg.ForwardClientCertDetails = envoyhttp.HttpConnectionManager_ForwardClientCertDetails(hcmSettings.ForwardClientCertDetails)

	if hcmSettings.AcceptHttp_10 {
		cfg.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{
			AcceptHttp_10:         true,
			DefaultHostForHttp_10: hcmSettings.DefaultHostForHttp_10,
		}
	}

	shouldConfigureClientCertDetails := (hcmSettings.ForwardClientCertDetails == hcm.HttpConnectionManagerSettings_APPEND_FORWARD ||
		hcmSettings.ForwardClientCertDetails == hcm.HttpConnectionManagerSettings_SANITIZE_SET) &&
		hcmSettings.SetCurrentClientCertDetails != nil

	if shouldConfigureClientCertDetails {
		cfg.SetCurrentClientCertDetails = &envoyhttp.HttpConnectionManager_SetCurrentClientCertDetails{
			Subject: gogoutils.BoolGogoToProto(hcmSettings.SetCurrentClientCertDetails.Subject),
			Cert:    hcmSettings.SetCurrentClientCertDetails.Cert,
			Chain:   hcmSettings.SetCurrentClientCertDetails.Chain,
			Dns:     hcmSettings.SetCurrentClientCertDetails.Dns,
			Uri:     hcmSettings.SetCurrentClientCertDetails.Uri,
		}
	}
}

var (
	hcmPluginError = func(err error) error {
		return errors.Wrapf(err, "error while running hcm plugin")
	}
)
