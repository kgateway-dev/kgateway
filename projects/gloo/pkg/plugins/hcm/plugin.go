package hcm

import (
	"context"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/upgradeconfig"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/go-utils/contextutils"
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
func (p *Plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoyapi.Listener) error {
	hl, ok := in.ListenerType.(*v1.Listener_HttpListener)
	if !ok {
		return nil
	}
	if hl.HttpListener == nil {
		return nil
	}
	hcmSettings := hl.HttpListener.GetOptions().GetHttpConnectionManagerSettings()
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
				if err := copyCoreHcmSettings(params.Ctx, &cfg, hcmSettings); err != nil {
					return err
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

func copyCoreHcmSettings(ctx context.Context, cfg *envoyhttp.HttpConnectionManager, hcmSettings *hcm.HttpConnectionManagerSettings) error {
	cfg.UseRemoteAddress = gogoutils.BoolGogoToProto(hcmSettings.GetUseRemoteAddress())
	cfg.XffNumTrustedHops = hcmSettings.GetXffNumTrustedHops()
	cfg.SkipXffAppend = hcmSettings.GetSkipXffAppend()
	cfg.Via = hcmSettings.GetVia()
	cfg.GenerateRequestId = gogoutils.BoolGogoToProto(hcmSettings.GetGenerateRequestId())
	cfg.Proxy_100Continue = hcmSettings.GetProxy_100Continue()
	cfg.StreamIdleTimeout = gogoutils.DurationStdToProto(hcmSettings.GetStreamIdleTimeout())
	cfg.MaxRequestHeadersKb = gogoutils.UInt32GogoToProto(hcmSettings.GetMaxRequestHeadersKb())
	cfg.RequestTimeout = gogoutils.DurationStdToProto(hcmSettings.GetRequestTimeout())
	cfg.DrainTimeout = gogoutils.DurationStdToProto(hcmSettings.GetDrainTimeout())
	cfg.DelayedCloseTimeout = gogoutils.DurationStdToProto(hcmSettings.GetDelayedCloseTimeout())
	cfg.ServerName = hcmSettings.GetServerName()
	cfg.PreserveExternalRequestId = hcmSettings.GetPreserveExternalRequestId()

	if hcmSettings.GetAcceptHttp_10() {
		cfg.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{
			AcceptHttp_10:         true,
			DefaultHostForHttp_10: hcmSettings.GetDefaultHostForHttp_10(),
		}
	}

	if hcmSettings.GetIdleTimeout() != nil {
		if cfg.GetCommonHttpProtocolOptions() == nil {
			cfg.CommonHttpProtocolOptions = &envoycore.HttpProtocolOptions{}
		}
		cfg.CommonHttpProtocolOptions.IdleTimeout = gogoutils.DurationStdToProto(hcmSettings.GetIdleTimeout())
	}

	// allowed upgrades
	protocolUpgrades := hcmSettings.GetUpgrades()

	webSocketUpgradeSpecified := false

	// try to catch
	// https://github.com/solo-io/gloo/issues/1979
	if len(cfg.UpgradeConfigs) != 0 {
		contextutils.LoggerFrom(ctx).DPanic("upgrade configs is not empty", "upgrade_configs", cfg.UpgradeConfigs)
	}

	cfg.UpgradeConfigs = make([]*envoyhttp.HttpConnectionManager_UpgradeConfig, len(protocolUpgrades))

	for i, config := range protocolUpgrades {
		switch upgradeType := config.UpgradeType.(type) {
		case *protocol_upgrade.ProtocolUpgradeConfig_Websocket:
			cfg.UpgradeConfigs[i] = &envoyhttp.HttpConnectionManager_UpgradeConfig{
				UpgradeType: upgradeconfig.WebSocketUpgradeType,
				Enabled:     gogoutils.BoolGogoToProto(config.GetWebsocket().GetEnabled()),
			}

			webSocketUpgradeSpecified = true
		default:
			return errors.Errorf("unimplemented upgrade type: %T", upgradeType)
		}
	}

	// enable websockets by default if no websocket upgrade was specified
	if !webSocketUpgradeSpecified {
		cfg.UpgradeConfigs = append(cfg.UpgradeConfigs, &envoyhttp.HttpConnectionManager_UpgradeConfig{
			UpgradeType: upgradeconfig.WebSocketUpgradeType,
		})
	}

	if err := upgradeconfig.ValidateHCMUpgradeConfigs(cfg.UpgradeConfigs); err != nil {
		return err
	}

	// client certificate forwarding
	cfg.ForwardClientCertDetails = envoyhttp.HttpConnectionManager_ForwardClientCertDetails(hcmSettings.GetForwardClientCertDetails())

	shouldConfigureClientCertDetails := (hcmSettings.GetForwardClientCertDetails() == hcm.HttpConnectionManagerSettings_APPEND_FORWARD ||
		hcmSettings.GetForwardClientCertDetails() == hcm.HttpConnectionManagerSettings_SANITIZE_SET) &&
		hcmSettings.GetSetCurrentClientCertDetails() != nil

	if shouldConfigureClientCertDetails {
		cfg.SetCurrentClientCertDetails = &envoyhttp.HttpConnectionManager_SetCurrentClientCertDetails{
			Subject: gogoutils.BoolGogoToProto(hcmSettings.GetSetCurrentClientCertDetails().GetSubject()),
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
