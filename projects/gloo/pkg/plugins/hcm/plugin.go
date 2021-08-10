package hcm

import (
	"context"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/upgradeconfig"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/go-utils/contextutils"
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.ListenerPlugin = new(Plugin)

type Plugin struct {
	hcmPlugins []HcmPlugin
}

func (p *Plugin) Init(_ plugins.InitParams) error {
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
func (p *Plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoy_config_listener_v3.Listener) error {
	hl, ok := in.GetListenerType().(*v1.Listener_HttpListener)
	if !ok {
		return nil
	}
	if hl.HttpListener == nil {
		return nil
	}
	hcmSettings := hl.HttpListener.GetOptions().GetHttpConnectionManagerSettings()
	for _, fc := range out.GetFilterChains() {
		for i, filter := range fc.GetFilters() {
			if filter.GetName() == wellknown.HTTPConnectionManager {
				// get config
				var cfg envoyhttp.HttpConnectionManager
				err := translatorutil.ParseTypedConfig(filter, &cfg)
				// this should never error
				if err != nil {
					return err
				}

				// first apply the core HCM settings, if any
				if err := copyCoreHcmSettings(params.Ctx, &cfg, hcmSettings); err != nil {
					return err
				}

				// then allow any HCM plugins to make their changes, with respect to any changes the core plugin made
				for _, hp := range p.hcmPlugins {
					if err := hp.ProcessHcmSettings(params.Snapshot, &cfg, hcmSettings); err != nil {
						return hcmPluginError(err)
					}
				}

				fc.GetFilters()[i], err = translatorutil.NewFilterWithTypedConfig(wellknown.HTTPConnectionManager, &cfg)
				// this should never error
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func copyCoreHcmSettings(ctx context.Context, cfg *envoyhttp.HttpConnectionManager, hcmSettings *hcm.HttpConnectionManagerSettings) error {
	cfg.UseRemoteAddress = hcmSettings.GetUseRemoteAddress()
	cfg.XffNumTrustedHops = hcmSettings.GetXffNumTrustedHops()
	cfg.SkipXffAppend = hcmSettings.GetSkipXffAppend()
	cfg.Via = hcmSettings.GetVia()
	cfg.GenerateRequestId = hcmSettings.GetGenerateRequestId()
	cfg.Proxy_100Continue = hcmSettings.GetProxy_100Continue()
	cfg.StreamIdleTimeout = hcmSettings.GetStreamIdleTimeout()
	cfg.MaxRequestHeadersKb = hcmSettings.GetMaxRequestHeadersKb()
	cfg.RequestTimeout = hcmSettings.GetRequestTimeout()
	cfg.DrainTimeout = hcmSettings.GetDrainTimeout()
	cfg.DelayedCloseTimeout = hcmSettings.GetDelayedCloseTimeout()
	cfg.ServerName = hcmSettings.GetServerName()
	cfg.PreserveExternalRequestId = hcmSettings.GetPreserveExternalRequestId()
	cfg.ServerHeaderTransformation = envoyhttp.HttpConnectionManager_ServerHeaderTransformation(hcmSettings.GetServerHeaderTransformation())
	cfg.PathWithEscapedSlashesAction = envoyhttp.HttpConnectionManager_PathWithEscapedSlashesAction(hcmSettings.GetPathWithEscapedSlashesAction())

	if hcmSettings.GetAcceptHttp_10() {
		cfg.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{
			AcceptHttp_10:         true,
			DefaultHostForHttp_10: hcmSettings.GetDefaultHostForHttp_10(),
		}
	}

	if hcmSettings.GetProperCaseHeaderKeyFormat() {
		if cfg.GetHttpProtocolOptions() == nil {
			cfg.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{}
		}
		cfg.GetHttpProtocolOptions().HeaderKeyFormat = &envoycore.Http1ProtocolOptions_HeaderKeyFormat{
			HeaderFormat: &envoycore.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords_{
				ProperCaseWords: &envoycore.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords{},
			},
		}
	}

	if hcmSettings.GetIdleTimeout() != nil {
		if cfg.GetCommonHttpProtocolOptions() == nil {
			cfg.CommonHttpProtocolOptions = &envoycore.HttpProtocolOptions{}
		}
		cfg.GetCommonHttpProtocolOptions().IdleTimeout = hcmSettings.GetIdleTimeout()
	}

	if hcmSettings.GetMaxConnectionDuration() != nil {
		if cfg.GetCommonHttpProtocolOptions() == nil {
			cfg.CommonHttpProtocolOptions = &envoycore.HttpProtocolOptions{}
		}
		cfg.GetCommonHttpProtocolOptions().MaxConnectionDuration = hcmSettings.GetMaxConnectionDuration()
	}

	if hcmSettings.GetMaxStreamDuration() != nil {
		if cfg.GetCommonHttpProtocolOptions() == nil {
			cfg.CommonHttpProtocolOptions = &envoycore.HttpProtocolOptions{}
		}
		cfg.GetCommonHttpProtocolOptions().MaxStreamDuration = hcmSettings.GetMaxStreamDuration()
	}

	// allowed upgrades
	protocolUpgrades := hcmSettings.GetUpgrades()

	webSocketUpgradeSpecified := false

	// try to catch
	// https://github.com/solo-io/gloo/issues/1979
	if len(cfg.GetUpgradeConfigs()) != 0 {
		contextutils.LoggerFrom(ctx).DPanic("upgrade configs is not empty", "upgrade_configs", cfg.GetUpgradeConfigs())
	}

	cfg.UpgradeConfigs = make([]*envoyhttp.HttpConnectionManager_UpgradeConfig, len(protocolUpgrades))

	for i, config := range protocolUpgrades {
		switch upgradeType := config.GetUpgradeType().(type) {
		case *protocol_upgrade.ProtocolUpgradeConfig_Websocket:
			cfg.GetUpgradeConfigs()[i] = &envoyhttp.HttpConnectionManager_UpgradeConfig{
				UpgradeType: upgradeconfig.WebSocketUpgradeType,
				Enabled:     config.GetWebsocket().GetEnabled(),
			}

			webSocketUpgradeSpecified = true
		default:
			return errors.Errorf("unimplemented upgrade type: %T", upgradeType)
		}
	}

	// enable websockets by default if no websocket upgrade was specified
	if !webSocketUpgradeSpecified {
		cfg.UpgradeConfigs = append(cfg.GetUpgradeConfigs(), &envoyhttp.HttpConnectionManager_UpgradeConfig{
			UpgradeType: upgradeconfig.WebSocketUpgradeType,
		})
	}

	if err := upgradeconfig.ValidateHCMUpgradeConfigs(cfg.GetUpgradeConfigs()); err != nil {
		return err
	}

	// client certificate forwarding
	cfg.ForwardClientCertDetails = envoyhttp.HttpConnectionManager_ForwardClientCertDetails(hcmSettings.GetForwardClientCertDetails())

	shouldConfigureClientCertDetails := (hcmSettings.GetForwardClientCertDetails() == hcm.HttpConnectionManagerSettings_APPEND_FORWARD ||
		hcmSettings.GetForwardClientCertDetails() == hcm.HttpConnectionManagerSettings_SANITIZE_SET) &&
		hcmSettings.GetSetCurrentClientCertDetails() != nil

	if shouldConfigureClientCertDetails {
		cfg.SetCurrentClientCertDetails = &envoyhttp.HttpConnectionManager_SetCurrentClientCertDetails{
			Subject: hcmSettings.GetSetCurrentClientCertDetails().GetSubject(),
			Cert:    hcmSettings.GetSetCurrentClientCertDetails().GetCert(),
			Chain:   hcmSettings.GetSetCurrentClientCertDetails().GetChain(),
			Dns:     hcmSettings.GetSetCurrentClientCertDetails().GetDns(),
			Uri:     hcmSettings.GetSetCurrentClientCertDetails().GetUri(),
		}
	}

	return nil
}

var (
	hcmPluginError = func(err error) error {
		return errors.Wrapf(err, "error while running hcm plugin")
	}
)
