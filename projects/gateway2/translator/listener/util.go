package listener

import (
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/solo-io/gloo/projects/gloo/constants"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	DefaultHttpStatPrefix = "http"
)

func initializeHCM(routeConfigName string) *envoyhttp.HttpConnectionManager {
	statPrefix := DefaultHttpStatPrefix

	return &envoyhttp.HttpConnectionManager{
		CodecType:     envoyhttp.HttpConnectionManager_AUTO,
		StatPrefix:    statPrefix,
		NormalizePath: wrapperspb.Bool(true),
		RouteSpecifier: &envoyhttp.HttpConnectionManager_Rds{
			Rds: &envoyhttp.Rds{
				ConfigSource: &envoy_config_core_v3.ConfigSource{
					ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
					ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_Ads{
						Ads: &envoy_config_core_v3.AggregatedConfigSource{},
					},
				},
				RouteConfigName: routeConfigName,
			},
		},
	}
}

func computeHttpFilters() []*envoyhttp.HttpFilter {
	var filters []*envoyhttp.HttpFilter
	routerV3 := routerv3.Router{}

	filters = append(filters, ProtoToHttpFilter(wellknown.Router, &routerV3))
	return filters
}

func ProtoToHttpFilter(name string, config proto.Message) *envoyhttp.HttpFilter {

	ret := new(envoyhttp.HttpFilter)
	ret.Name = name
	ret.ConfigType = &envoyhttp.HttpFilter_TypedConfig{
		TypedConfig: toAny(config),
	}

	return ret

}

func toAny(pb proto.Message) *anypb.Any {
	any, err := anypb.New(pb)
	if err != nil {
		// all config types should already be known
		// therefore this should never happen
		panic(err)
	}
	return any
}

func makeFilterChain(info *FilterChainInfo, config *envoyhttp.HttpConnectionManager) *listenerv3.FilterChain {
	return &listenerv3.FilterChain{
		FilterChainMatch: toMatch(info),
		TransportSocket:  toTransportSocket(info),
		Filters: []*listenerv3.Filter{
			{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listenerv3.Filter_TypedConfig{
					TypedConfig: toAny(config),
				},
			},
		},
	}
}

func toMatch(info *FilterChainInfo) *listenerv3.FilterChainMatch {
	if info == nil {
		return nil
	}
	return &listenerv3.FilterChainMatch{
		ServerNames: info.SslConfig.SniDomains,
	}
}
func toTransportSocket(info *FilterChainInfo) *corev3.TransportSocket {
	if info == nil {
		return nil
	}
	ssl := info.SslConfig
	if ssl == nil {
		return nil
	}

	common := &envoyauth.CommonTlsContext{
		// default params
		TlsParams: &envoyauth.TlsParameters{},
	}

	common.TlsCertificates = []*envoyauth.TlsCertificate{
		{
			CertificateChain: bytesDataSource(ssl.Bundle.CertChain),
			PrivateKey:       bytesDataSource(ssl.Bundle.PrivateKey),
		},
	}

	//	var requireClientCert *wrappers.BoolValue
	//	if common.GetValidationContextType() != nil {
	//		requireClientCert = &wrappers.BoolValue{Value: !dc.GetOneWayTls().GetValue()}
	//	}

	// default alpn for downstreams.
	if len(common.GetAlpnProtocols()) == 0 {
		common.AlpnProtocols = []string{"h2", "http/1.1"}
	} else if len(common.GetAlpnProtocols()) == 1 && common.GetAlpnProtocols()[0] == constants.AllowEmpty { // allow override for advanced usage to set to a dangerous setting
		common.AlpnProtocols = []string{}

	}

	out := &envoyauth.DownstreamTlsContext{
		CommonTlsContext: common,
		//		RequireClientCertificate: requireClientCert,
	}
	typedConfig := toAny(out)

	return &envoy_config_core_v3.TransportSocket{
		Name:       wellknown.TransportSocketTls,
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}
}

func newRouteConfig(routeConfigName string, vhostsForFilterchain []*routev3.VirtualHost) *routev3.RouteConfiguration {
	rc := &routev3.RouteConfiguration{
		Name:                     routeConfigName,
		IgnorePortInHostMatching: true,
		VirtualHosts:             vhostsForFilterchain,
	}
	return rc
}
func bytesDataSource(s []byte) *corev3.DataSource {
	return &corev3.DataSource{
		Specifier: &corev3.DataSource_InlineBytes{
			InlineBytes: s,
		},
	}

}
func computeListenerAddress(bindAddress string, port uint32) *envoy_config_core_v3.Address {
	_, isIpv4Address, err := translator.IsIpv4Address(bindAddress)
	if err != nil {
		// TODO: ????
		// validation.AppendListenerError(l.report,
		// 	validationapi.ListenerReport_Error_ProcessingError,
		// 	err.Error(),
		// )
	}

	return &envoy_config_core_v3.Address{
		Address: &envoy_config_core_v3.Address_SocketAddress{
			SocketAddress: &envoy_config_core_v3.SocketAddress{
				Protocol: envoy_config_core_v3.SocketAddress_TCP,
				Address:  bindAddress,
				PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
					PortValue: port,
				},
				// As of Envoy 1.22: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.22/v1.22.0.html
				// the Ipv4Compat flag can only be set on Ipv6 address and Ipv4-mapped Ipv6 address.
				// Check if this is a non-padded pure ipv4 address and unset the compat flag if so.
				Ipv4Compat: !isIpv4Address,
			},
		},
	}
}
