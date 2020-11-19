package internal

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_config_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_filter_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	envoy_config_listener_v2 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v2"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
)

func DowngradeListener(listener *envoy_config_listener_v3.Listener) *envoyapi.Listener {
	downgradedListener := &envoyapi.Listener{
		Name:                          listener.GetName(),
		Address:                       downgradeAddress(listener.GetAddress()),
		FilterChains:                  nil,
		UseOriginalDst:                listener.GetHiddenEnvoyDeprecatedUseOriginalDst(),
		PerConnectionBufferLimitBytes: listener.GetPerConnectionBufferLimitBytes(),
		Metadata:                      downgradeMetadata(listener.GetMetadata()),
		DrainType: envoyapi.Listener_DrainType(
			envoyapi.Listener_DrainType_value[listener.GetDrainType().String()],
		),
		ListenerFilters: make(
			[]*envoy_api_v2_listener.ListenerFilter, 0, len(listener.GetListenerFilters()),
		),
		ListenerFiltersTimeout:           listener.GetListenerFiltersTimeout(),
		ContinueOnListenerFiltersTimeout: listener.GetContinueOnListenerFiltersTimeout(),
		Transparent:                      listener.GetTransparent(),
		Freebind:                         listener.GetFreebind(),
		SocketOptions:                    make([]*envoycore.SocketOption, 0, len(listener.GetSocketOptions())),
		TcpFastOpenQueueLength:           listener.GetTcpFastOpenQueueLength(),
		TrafficDirection: envoycore.TrafficDirection(
			envoycore.TrafficDirection_value[listener.GetTrafficDirection().String()],
		),
		ApiListener: &envoy_config_listener_v2.ApiListener{
			ApiListener: listener.GetApiListener().GetApiListener(),
		},
		ReusePort: listener.GetReusePort(),
		AccessLog: make([]*envoy_config_filter_accesslog_v2.AccessLog, 0, len(listener.GetAccessLog())),
		// Fields which are unused by gloo
		DeprecatedV1:            nil,
		UdpListenerConfig:       nil,
		ConnectionBalanceConfig: nil,
	}

	for _, v := range listener.GetListenerFilters() {
		downgradedListener.ListenerFilters = append(downgradedListener.ListenerFilters, downgradeListenerFilter(v))
	}

	for _, v := range listener.GetSocketOptions() {
		downgradedListener.SocketOptions = append(downgradedListener.SocketOptions, downgradeSocketOption(v))
	}

	for _, v := range listener.GetAccessLog() {
		downgradedListener.AccessLog = append(downgradedListener.AccessLog, downgradeAccessLog(v))
	}

	return downgradedListener
}

func downgradeFitlerChain(filter *envoy_config_listener_v3.FilterChain) *envoy_api_v2_listener.FilterChain {
	downgradedFilterChain := &envoy_api_v2_listener.FilterChain{
		FilterChainMatch: downgradeFilterChainMatch(filter.GetFilterChainMatch()),
		TlsContext:       nil,
		Filters:          make([]*envoy_api_v2_listener.Filter, 0, len(filter.GetFilters())),
		UseProxyProto:    filter.GetUseProxyProto(),
		Metadata:         downgradeMetadata(filter.GetMetadata()),
		Name:             filter.GetName(),
		TransportSocket: downgradeTransportSocket(filter.GetTransportSocket()),
	}

	for _, v := range filter.GetFilters() {
		downgradedFilterChain.Filters = append(downgradedFilterChain.Filters, downgradeFilter(v))
	}
	return downgradedFilterChain
}

func downgradeTransportSocket(ts *envoy_config_core_v3.TransportSocket) *envoycore.TransportSocket {
	if ts == nil {
		return nil
	}
	return &envoycore.TransportSocket{
		Name: ts.GetName(),
		ConfigType: &envoycore.TransportSocket_TypedConfig{
			TypedConfig: ts.GetTypedConfig(),
		},
	}
}

func downgradeFilterChainMatch(match *envoy_config_listener_v3.FilterChainMatch) *envoy_api_v2_listener.FilterChainMatch {
	downgradedMatch := &envoy_api_v2_listener.FilterChainMatch{
		DestinationPort: match.GetDestinationPort(),
		PrefixRanges:    make([]*envoycore.CidrRange, 0, len(match.GetPrefixRanges())),
		AddressSuffix:   match.GetAddressSuffix(),
		SuffixLen:       match.GetSuffixLen(),
		SourceType: envoy_api_v2_listener.FilterChainMatch_ConnectionSourceType(
			envoy_api_v2_listener.FilterChainMatch_ConnectionSourceType_value[match.GetSourceType().String()],
		),
		SourcePrefixRanges:   make([]*envoycore.CidrRange, 0, len(match.GetSourcePrefixRanges())),
		SourcePorts:          match.GetSourcePorts(),
		ServerNames:          match.GetServerNames(),
		TransportProtocol:    match.GetTransportProtocol(),
		ApplicationProtocols: match.GetApplicationProtocols(),
	}

	for _, v := range match.GetPrefixRanges() {
		downgradedMatch.PrefixRanges = append(downgradedMatch.PrefixRanges, downgradeRange(v))
	}

	for _, v := range match.GetSourcePrefixRanges() {
		downgradedMatch.SourcePrefixRanges = append(downgradedMatch.SourcePrefixRanges, downgradeRange(v))
	}

	return downgradedMatch
}

func downgradeRange(rng *envoy_config_core_v3.CidrRange) *envoycore.CidrRange {
	if rng == nil {
		return nil
	}
	return &envoycore.CidrRange{
		AddressPrefix: rng.GetAddressPrefix(),
		PrefixLen:     rng.GetPrefixLen(),
	}
}

func downgradeFilter(filter *envoy_config_listener_v3.Filter) *envoy_api_v2_listener.Filter {
	if filter == nil {
		return nil
	}
	return &envoy_api_v2_listener.Filter{
		Name: filter.GetName(),
		ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
			TypedConfig: filter.GetTypedConfig(),
		},
	}
}

func downgradeAccessLog(al *envoy_config_accesslog_v3.AccessLog) *envoy_config_filter_accesslog_v2.AccessLog {
	if al == nil {
		return nil
	}
	return &envoy_config_filter_accesslog_v2.AccessLog{
		Name: al.GetName(),
		// Unsupported by Gloo
		Filter: nil,
		ConfigType: &envoy_config_filter_accesslog_v2.AccessLog_TypedConfig{
			TypedConfig: al.GetTypedConfig(),
		},
	}

}

func downgradeSocketOption(opt *envoy_config_core_v3.SocketOption) *envoycore.SocketOption {
	downgradedOption := &envoycore.SocketOption{
		Description: opt.GetDescription(),
		Level:       opt.GetLevel(),
		Name:        opt.GetName(),
		State: envoycore.SocketOption_SocketState(
			envoycore.SocketOption_SocketState_value[opt.GetState().String()],
		),
	}

	switch opt.GetValue().(type) {
	case *envoy_config_core_v3.SocketOption_BufValue:
		downgradedOption.Value = &envoycore.SocketOption_BufValue{
			BufValue: opt.GetBufValue(),
		}
	case *envoy_config_core_v3.SocketOption_IntValue:
		downgradedOption.Value = &envoycore.SocketOption_IntValue{
			IntValue: opt.GetIntValue(),
		}
	}

	return downgradedOption
}

func downgradeMetadata(meta *envoy_config_core_v3.Metadata) *envoycore.Metadata {
	return &envoycore.Metadata{
		FilterMetadata: meta.GetFilterMetadata(),
	}
}

func downgradeListenerFilter(filter *envoy_config_listener_v3.ListenerFilter) *envoy_api_v2_listener.ListenerFilter {
	if filter == nil {
		return nil
	}
	return &envoy_api_v2_listener.ListenerFilter{
		Name: filter.GetName(),
		ConfigType: &envoy_api_v2_listener.ListenerFilter_TypedConfig{
			TypedConfig: filter.GetTypedConfig(),
		},
		// Skipping for now as we don't expose this field in our API, and it's recursion makes it more complex
		FilterDisabled: nil,
	}
}

func downgradeAddress(address *envoy_config_core_v3.Address) *envoycore.Address {
	var downgradedAddress *envoycore.Address

	switch typed := address.GetAddress().(type) {
	case *envoy_config_core_v3.Address_SocketAddress:
		downgradedAddress = &envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: downgradeSocketAddress(typed.SocketAddress),
			},
		}
	case *envoy_config_core_v3.Address_Pipe:
		downgradedAddress = &envoycore.Address{
			Address: &envoycore.Address_Pipe{
				Pipe: &envoycore.Pipe{
					Path: typed.Pipe.GetPath(),
					Mode: typed.Pipe.GetMode(),
				},
			},
		}
	}

	return downgradedAddress
}

func downgradeSocketAddress(address *envoy_config_core_v3.SocketAddress) *envoycore.SocketAddress {
	if address == nil {
		return nil
	}

	socketAddress := &envoycore.SocketAddress{
		Protocol: envoycore.SocketAddress_Protocol(
			envoycore.SocketAddress_Protocol_value[address.GetProtocol().String()],
		),
		Address:      address.GetAddress(),
		ResolverName: address.GetResolverName(),
		Ipv4Compat:   address.GetIpv4Compat(),
	}
	switch address.GetPortSpecifier().(type) {
	case *envoy_config_core_v3.SocketAddress_PortValue:
		socketAddress.PortSpecifier = &envoycore.SocketAddress_PortValue{
			PortValue: address.GetPortValue(),
		}
	case *envoy_config_core_v3.SocketAddress_NamedPort:
		socketAddress.PortSpecifier = &envoycore.SocketAddress_NamedPort{
			NamedPort: address.GetNamedPort(),
		}
	}
	return socketAddress
}
