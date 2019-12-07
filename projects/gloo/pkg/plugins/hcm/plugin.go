package hcm

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

const (
	webSocketUpgradeType = "websocket"
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
			if filter.Name == envoyutil.HTTPConnectionManager {
				// get config
				var cfg envoyhttp.HttpConnectionManager
				err := translatorutil.ParseConfig(&filter, &cfg)
				// this should never error
				if err != nil {
					return err
				}

				// first apply the core HCM settings, if any
				if hcmSettings != nil {
					if err := copyCoreHcmSettings(&cfg, hcmSettings); err != nil {
						return err
					}
				}

				// then allow any HCM plugins to make their changes, with respect to any changes the core plugin made
				for _, hp := range p.hcmPlugins {
					if err := hp.ProcessHcmSettings(&cfg, hcmSettings); err != nil {
						return hcmPluginError(err)
					}
				}

				f.Filters[i], err = translatorutil.NewFilterWithConfig(envoyutil.HTTPConnectionManager, &cfg)
				// this should never error
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func copyCoreHcmSettings(cfg *envoyhttp.HttpConnectionManager, hcmSettings *hcm.HttpConnectionManagerSettings) error {
	cfg.UseRemoteAddress = hcmSettings.UseRemoteAddress
	cfg.XffNumTrustedHops = hcmSettings.XffNumTrustedHops
	cfg.SkipXffAppend = hcmSettings.SkipXffAppend
	cfg.Via = hcmSettings.Via
	cfg.GenerateRequestId = hcmSettings.GenerateRequestId
	cfg.Proxy_100Continue = hcmSettings.Proxy_100Continue
	cfg.StreamIdleTimeout = hcmSettings.StreamIdleTimeout
	cfg.IdleTimeout = hcmSettings.IdleTimeout
	cfg.MaxRequestHeadersKb = hcmSettings.MaxRequestHeadersKb
	cfg.RequestTimeout = hcmSettings.RequestTimeout
	cfg.DrainTimeout = hcmSettings.DrainTimeout
	cfg.DelayedCloseTimeout = hcmSettings.DelayedCloseTimeout
	cfg.ServerName = hcmSettings.ServerName

	if hcmSettings.AcceptHttp_10 {
		cfg.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{
			AcceptHttp_10:         true,
			DefaultHostForHttp_10: hcmSettings.DefaultHostForHttp_10,
		}
	}

	// allowed upgrades
	protocolUpgrades := hcmSettings.GetUpgrades()

	webSocketUpgradeSpecified := false

	if protocolUpgrades != nil {
		cfg.UpgradeConfigs = make([]*envoyhttp.HttpConnectionManager_UpgradeConfig, len(protocolUpgrades))

		for i, config := range protocolUpgrades {
			switch upgradeType := config.UpgradeType.(type) {
			case *protocol_upgrade.ProtocolUpgradeConfig_Websocket:
				cfg.UpgradeConfigs[i] = &envoyhttp.HttpConnectionManager_UpgradeConfig{
					UpgradeType: webSocketUpgradeType,
					Enabled:     config.GetWebsocket().GetEnabled(),
				}

				webSocketUpgradeSpecified = true
			default:
				return errors.Errorf("unimplemented upgrade type: %T", upgradeType)
			}
		}
	}

	// enable websockets by default if no websocket upgrade was specified
	if !webSocketUpgradeSpecified {
		cfg.UpgradeConfigs = append(cfg.UpgradeConfigs, &envoyhttp.HttpConnectionManager_UpgradeConfig{
			UpgradeType: webSocketUpgradeType,
		})
	}

	// client certificate forwarding
	cfg.ForwardClientCertDetails = envoyhttp.HttpConnectionManager_ForwardClientCertDetails(hcmSettings.ForwardClientCertDetails)

	shouldConfigureClientCertDetails := (hcmSettings.ForwardClientCertDetails == hcm.HttpConnectionManagerSettings_APPEND_FORWARD ||
		hcmSettings.ForwardClientCertDetails == hcm.HttpConnectionManagerSettings_SANITIZE_SET) &&
		hcmSettings.SetCurrentClientCertDetails != nil

	if shouldConfigureClientCertDetails {
		cfg.SetCurrentClientCertDetails = &envoyhttp.HttpConnectionManager_SetCurrentClientCertDetails{
			Subject: hcmSettings.SetCurrentClientCertDetails.Subject,
			Cert:    hcmSettings.SetCurrentClientCertDetails.Cert,
			Chain:   hcmSettings.SetCurrentClientCertDetails.Chain,
			Dns:     hcmSettings.SetCurrentClientCertDetails.Dns,
			Uri:     hcmSettings.SetCurrentClientCertDetails.Uri,
		}
	}

	return nil
}

var (
	hcmPluginError = func(err error) error {
		return errors.Wrapf(err, "error while running hcm plugin")
	}
)
