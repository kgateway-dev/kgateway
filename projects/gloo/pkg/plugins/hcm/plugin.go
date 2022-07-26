package hcm

import (
	"fmt"
	"net"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_extensions_http_header_formatters_preserve_case_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/header_formatters/preserve_case/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/httpprotocolvalidation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/upgradeconfig"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	_ plugins.Plugin                      = new(plugin)
	_ plugins.HttpConnectionManagerPlugin = new(plugin)
)

const (
	ExtensionName      = "hcm"
	PreserveCasePlugin = "envoy.http.stateful_header_formatters.preserve_case"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) ProcessHcmNetworkFilter(params plugins.Params, _ *v1.Listener, listener *v1.HttpListener, out *envoyhttp.HttpConnectionManager) error {
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
	out.RequestHeadersTimeout = in.GetRequestHeadersTimeout()
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

	// if we want to set a header format with `in`, ensure `out` has a non-nil value
	if in.GetHeaderFormat() != nil && out.GetHttpProtocolOptions() == nil {
		out.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{}
	}
	if in.GetProperCaseHeaderKeyFormat() {
		out.GetHttpProtocolOptions().HeaderKeyFormat = &envoycore.Http1ProtocolOptions_HeaderKeyFormat{
			HeaderFormat: &envoycore.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords_{
				ProperCaseWords: &envoycore.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords{},
			},
		}
	} else if in.GetPreserveCaseHeaderKeyFormat() {
		out.GetHttpProtocolOptions().HeaderKeyFormat = &envoycore.Http1ProtocolOptions_HeaderKeyFormat{
			HeaderFormat: &envoycore.Http1ProtocolOptions_HeaderKeyFormat_StatefulFormatter{
				StatefulFormatter: &envoycore.TypedExtensionConfig{
					Name:        PreserveCasePlugin,
					TypedConfig: utils.MustMessageToAny(&envoy_extensions_http_header_formatters_preserve_case_v3.PreserveCaseFormatterConfig{}),
				},
			},
		}
	}

	if in.GetInternalAddressConfig() != nil {
	 	if out.GetInternalAddressConfig() == nil {
	 		out.InternalAddressConfig = &envoyhttp.HttpConnectionManager_InternalAddressConfig{}
	 	}
		out.InternalAddressConfig.UnixSockets = in.InternalAddressConfig.UnixSockets
		for _, cidrRange := range in.InternalAddressConfig.CidrRanges {
			_, _, err := net.ParseCIDR(fmt.Sprintf("%s/%d", cidrRange.AddressPrefix, cidrRange.PrefixLen.Value))
			if err != nil {
				return err
			}
			out.InternalAddressConfig.CidrRanges = append(out.InternalAddressConfig.CidrRanges, &envoycore.CidrRange{
				AddressPrefix: cidrRange.AddressPrefix,
				PrefixLen: cidrRange.PrefixLen,
			})
		}
	}

	if in.GetAllowChunkedLength() {
		if out.GetHttpProtocolOptions() == nil {
			out.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{}
		}
		out.GetHttpProtocolOptions().AllowChunkedLength = in.GetAllowChunkedLength()
	}

	if in.GetEnableTrailers() {
		if out.GetHttpProtocolOptions() == nil {
			out.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{}
		}

		out.GetHttpProtocolOptions().EnableTrailers = in.GetEnableTrailers()
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

	if in.GetMaxRequestsPerConnection() != nil {
		if out.GetCommonHttpProtocolOptions() == nil {
			out.CommonHttpProtocolOptions = &envoycore.HttpProtocolOptions{}
		}
		out.GetCommonHttpProtocolOptions().MaxRequestsPerConnection = in.GetMaxRequestsPerConnection()
	}

	if in.GetHeadersWithUnderscoresAction() != hcm.HttpConnectionManagerSettings_ALLOW {
		if out.GetCommonHttpProtocolOptions() == nil {
			out.CommonHttpProtocolOptions = &envoycore.HttpProtocolOptions{}
		}
		out.GetCommonHttpProtocolOptions().HeadersWithUnderscoresAction = envoycore.HttpProtocolOptions_HeadersWithUnderscoresAction(in.GetHeadersWithUnderscoresAction())
	}

	if in.GetStripAnyHostPort() {
		if out.GetStripPortMode() == nil {
			out.StripPortMode = &envoyhttp.HttpConnectionManager_StripAnyHostPort{
				StripAnyHostPort: true,
			}
		}
	}

	if in.GetUuidRequestIdConfig() != nil {
		// Create a new empty request id extension if none present
		if out.GetRequestIdExtension() == nil {
			out.RequestIdExtension = &envoyhttp.RequestIDExtension{}
		}

		var err error
		// No errors should occur when marshaling
		if out.GetRequestIdExtension().TypedConfig, err = anypb.New(in.GetUuidRequestIdConfig()); err != nil {
			return err
		}
	}

	if in.GetHttp2ProtocolOptions() != nil {
		http2po := in.GetHttp2ProtocolOptions()
		// Both these values default to 268435456 if unset.
		sws := http2po.GetInitialStreamWindowSize()
		if sws != nil {
			if !httpprotocolvalidation.ValidateWindowSize(sws.GetValue()) {
				return errors.Errorf("Invalid Initial Stream Window Size: %d", sws.GetValue())
			} else {
				sws = &wrappers.UInt32Value{Value: sws.GetValue()}
			}
		}

		cws := http2po.GetInitialConnectionWindowSize()
		if cws != nil {
			if !httpprotocolvalidation.ValidateWindowSize(cws.GetValue()) {
				return errors.Errorf("Invalid Initial Connection Window Size: %d", cws.GetValue())
			} else {
				cws = &wrappers.UInt32Value{Value: cws.GetValue()}
			}
		}

		mcs := http2po.GetMaxConcurrentStreams()
		if mcs != nil {
			if !httpprotocolvalidation.ValidateConcurrentStreams(mcs.GetValue()) {
				return errors.Errorf("Invalid Max Concurrent Streams Size: %d", mcs.GetValue())
			} else {
				mcs = &wrappers.UInt32Value{Value: mcs.GetValue()}
			}
		}

		ose := http2po.GetOverrideStreamErrorOnInvalidHttpMessage()

		out.Http2ProtocolOptions = &envoycore.Http2ProtocolOptions{
			InitialStreamWindowSize:                 sws,
			InitialConnectionWindowSize:             cws,
			MaxConcurrentStreams:                    mcs,
			OverrideStreamErrorOnInvalidHttpMessage: ose,
		}
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
