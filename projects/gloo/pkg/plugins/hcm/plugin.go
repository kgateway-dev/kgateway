package hcm

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/upgradeconfig"
	"github.com/solo-io/go-utils/contextutils"
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.HttpConnectionManagerPlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(_ plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessHcmNetworkFilter(params plugins.Params, _ *v1.Listener, listener *v1.HttpListener, out *envoyhttp.HttpConnectionManager) error {
	in := listener.GetOptions().GetHttpConnectionManagerSettings()

	out.UseRemoteAddress = in.GetUseRemoteAddress()
	out.XffNumTrustedHops = in.GetXffNumTrustedHops()
	out.SkipXffAppend = in.GetSkipXffAppend()
	out.Via = in.GetVia()
	out.GenerateRequestId = in.GetGenerateRequestId()
	out.Proxy_100Continue = in.GetProxy_100Continue()
	out.StreamIdleTimeout = in.GetStreamIdleTimeout()
	out.MaxRequestHeadersKb = in.GetMaxRequestHeadersKb()
	out.RequestTimeout = in.GetRequestTimeout()
	out.DrainTimeout = in.GetDrainTimeout()
	out.DelayedCloseTimeout = in.GetDelayedCloseTimeout()
	out.ServerName = in.GetServerName()
	out.PreserveExternalRequestId = in.GetPreserveExternalRequestId()
	out.ServerHeaderTransformation = envoyhttp.HttpConnectionManager_ServerHeaderTransformation(in.GetServerHeaderTransformation())
	out.PathWithEscapedSlashesAction = envoyhttp.HttpConnectionManager_PathWithEscapedSlashesAction(in.GetPathWithEscapedSlashesAction())
	out.CodecType = envoyhttp.HttpConnectionManager_CodecType(in.GetCodecType())
	out.MergeSlashes = in.GetMergeSlashes()
	out.NormalizePath = in.GetNormalizePath()

	if in.GetAcceptHttp_10() {
		out.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{
			AcceptHttp_10:         true,
			DefaultHostForHttp_10: in.GetDefaultHostForHttp_10(),
		}
	}

	if in.GetProperCaseHeaderKeyFormat() {
		if out.GetHttpProtocolOptions() == nil {
			out.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{}
		}
		out.GetHttpProtocolOptions().HeaderKeyFormat = &envoycore.Http1ProtocolOptions_HeaderKeyFormat{
			HeaderFormat: &envoycore.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords_{
				ProperCaseWords: &envoycore.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords{},
			},
		}
	}

	if in.GetIdleTimeout() != nil {
		if out.GetCommonHttpProtocolOptions() == nil {
			out.CommonHttpProtocolOptions = &envoycore.HttpProtocolOptions{}
		}
		out.GetCommonHttpProtocolOptions().IdleTimeout = in.GetIdleTimeout()
	}

	if in.GetMaxConnectionDuration() != nil {
		if out.GetCommonHttpProtocolOptions() == nil {
			out.CommonHttpProtocolOptions = &envoycore.HttpProtocolOptions{}
		}
		out.GetCommonHttpProtocolOptions().MaxConnectionDuration = in.GetMaxConnectionDuration()
	}

	if in.GetMaxStreamDuration() != nil {
		if out.GetCommonHttpProtocolOptions() == nil {
			out.CommonHttpProtocolOptions = &envoycore.HttpProtocolOptions{}
		}
		out.GetCommonHttpProtocolOptions().MaxStreamDuration = in.GetMaxStreamDuration()
	}

	if in.GetMaxHeadersCount() != nil {
		if out.GetCommonHttpProtocolOptions() == nil {
			out.CommonHttpProtocolOptions = &envoycore.HttpProtocolOptions{}
		}
		out.GetCommonHttpProtocolOptions().MaxHeadersCount = in.GetMaxHeadersCount()
	}

	// allowed upgrades
	protocolUpgrades := in.GetUpgrades()

	webSocketUpgradeSpecified := false

	// try to catch
	// https://github.com/solo-io/gloo/issues/1979
	if len(out.GetUpgradeConfigs()) != 0 {
		contextutils.LoggerFrom(params.Ctx).DPanic("upgrade configs is not empty", "upgrade_configs", out.GetUpgradeConfigs())
	}

	out.UpgradeConfigs = make([]*envoyhttp.HttpConnectionManager_UpgradeConfig, len(protocolUpgrades))

	for i, config := range protocolUpgrades {
		switch upgradeType := config.GetUpgradeType().(type) {
		case *protocol_upgrade.ProtocolUpgradeConfig_Websocket:
			out.GetUpgradeConfigs()[i] = &envoyhttp.HttpConnectionManager_UpgradeConfig{
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
		out.UpgradeConfigs = append(out.GetUpgradeConfigs(), &envoyhttp.HttpConnectionManager_UpgradeConfig{
			UpgradeType: upgradeconfig.WebSocketUpgradeType,
		})
	}

	if err := upgradeconfig.ValidateHCMUpgradeConfigs(out.GetUpgradeConfigs()); err != nil {
		return err
	}

	// client certificate forwarding
	out.ForwardClientCertDetails = envoyhttp.HttpConnectionManager_ForwardClientCertDetails(in.GetForwardClientCertDetails())

	shouldConfigureClientCertDetails := (in.GetForwardClientCertDetails() == hcm.HttpConnectionManagerSettings_APPEND_FORWARD ||
		in.GetForwardClientCertDetails() == hcm.HttpConnectionManagerSettings_SANITIZE_SET) &&
		in.GetSetCurrentClientCertDetails() != nil

	if shouldConfigureClientCertDetails {
		out.SetCurrentClientCertDetails = &envoyhttp.HttpConnectionManager_SetCurrentClientCertDetails{
			Subject: in.GetSetCurrentClientCertDetails().GetSubject(),
			Cert:    in.GetSetCurrentClientCertDetails().GetCert(),
			Chain:   in.GetSetCurrentClientCertDetails().GetChain(),
			Dns:     in.GetSetCurrentClientCertDetails().GetDns(),
			Uri:     in.GetSetCurrentClientCertDetails().GetUri(),
		}
	}

	return nil

}
