package internal

import (
	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_config_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_filter_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	envoy_config_listener_v2 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v2"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
)

func DowngradeListener(listener *envoy_config_listener_v3.Listener) *envoy_api_v2.Listener {
	if listener == nil {
		return nil
	}

	downgradedListener := &envoy_api_v2.Listener{
		Name:                          listener.GetName(),
		Address:                       downgradeAddress(listener.GetAddress()),
		UseOriginalDst:                listener.GetHiddenEnvoyDeprecatedUseOriginalDst(),
		PerConnectionBufferLimitBytes: listener.GetPerConnectionBufferLimitBytes(),
		Metadata:                      downgradeMetadata(listener.GetMetadata()),
		DrainType: envoy_api_v2.Listener_DrainType(
			envoy_api_v2.Listener_DrainType_value[listener.GetDrainType().String()],
		),
		ListenerFilters: make(
			[]*envoy_api_v2_listener.ListenerFilter, 0, len(listener.GetListenerFilters()),
		),
		ListenerFiltersTimeout:           listener.GetListenerFiltersTimeout(),
		ContinueOnListenerFiltersTimeout: listener.GetContinueOnListenerFiltersTimeout(),
		Transparent:                      listener.GetTransparent(),
		Freebind:                         listener.GetFreebind(),
		SocketOptions:                    make([]*envoy_api_v2_core.SocketOption, 0, len(listener.GetSocketOptions())),
		TcpFastOpenQueueLength:           listener.GetTcpFastOpenQueueLength(),
		TrafficDirection: envoy_api_v2_core.TrafficDirection(
			envoy_api_v2_core.TrafficDirection_value[listener.GetTrafficDirection().String()],
		),
		ApiListener: downgradeApiListener(listener.GetApiListener()),
		ReusePort:   listener.GetReusePort(),
		AccessLog:   make([]*envoy_config_filter_accesslog_v2.AccessLog, 0, len(listener.GetAccessLog())),
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

	for _, v := range listener.GetFilterChains() {
		downgradedListener.FilterChains = append(downgradedListener.FilterChains, downgradeFitlerChain(v))
	}

	return downgradedListener
}

func downgradeApiListener(list *envoy_config_listener_v3.ApiListener) *envoy_config_listener_v2.ApiListener {
	if list == nil {
		return nil
	}

	return &envoy_config_listener_v2.ApiListener{
		ApiListener: list.GetApiListener(),
	}
}

func downgradeFitlerChain(filter *envoy_config_listener_v3.FilterChain) *envoy_api_v2_listener.FilterChain {
	if filter == nil {
		return nil
	}
	downgradedFilterChain := &envoy_api_v2_listener.FilterChain{
		FilterChainMatch: downgradeFilterChainMatch(filter.GetFilterChainMatch()),
		TlsContext:       nil,
		Filters:          make([]*envoy_api_v2_listener.Filter, 0, len(filter.GetFilters())),
		UseProxyProto:    filter.GetUseProxyProto(),
		Metadata:         downgradeMetadata(filter.GetMetadata()),
		Name:             filter.GetName(),
		TransportSocket:  downgradeTransportSocket(filter.GetTransportSocket()),
	}

	for _, v := range filter.GetFilters() {
		downgradedFilterChain.Filters = append(downgradedFilterChain.Filters, downgradeFilter(v))
	}
	return downgradedFilterChain
}

func downgradeTransportSocket(ts *envoy_config_core_v3.TransportSocket) *envoy_api_v2_core.TransportSocket {
	if ts == nil {
		return nil
	}
	return &envoy_api_v2_core.TransportSocket{
		Name: ts.GetName(),
		ConfigType: &envoy_api_v2_core.TransportSocket_TypedConfig{
			TypedConfig: ts.GetTypedConfig(),
		},
	}
}

func downgradeFilterChainMatch(match *envoy_config_listener_v3.FilterChainMatch) *envoy_api_v2_listener.FilterChainMatch {
	if match == nil {
		return nil
	}
	downgradedMatch := &envoy_api_v2_listener.FilterChainMatch{
		DestinationPort: match.GetDestinationPort(),
		PrefixRanges:    make([]*envoy_api_v2_core.CidrRange, 0, len(match.GetPrefixRanges())),
		AddressSuffix:   match.GetAddressSuffix(),
		SuffixLen:       match.GetSuffixLen(),
		SourceType: envoy_api_v2_listener.FilterChainMatch_ConnectionSourceType(
			envoy_api_v2_listener.FilterChainMatch_ConnectionSourceType_value[match.GetSourceType().String()],
		),
		SourcePrefixRanges:   make([]*envoy_api_v2_core.CidrRange, 0, len(match.GetSourcePrefixRanges())),
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

func downgradeRange(rng *envoy_config_core_v3.CidrRange) *envoy_api_v2_core.CidrRange {
	if rng == nil {
		return nil
	}
	return &envoy_api_v2_core.CidrRange{
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

func downgradeSocketOption(opt *envoy_config_core_v3.SocketOption) *envoy_api_v2_core.SocketOption {
	if opt == nil {
		return nil
	}
	downgradedOption := &envoy_api_v2_core.SocketOption{
		Description: opt.GetDescription(),
		Level:       opt.GetLevel(),
		Name:        opt.GetName(),
		State: envoy_api_v2_core.SocketOption_SocketState(
			envoy_api_v2_core.SocketOption_SocketState_value[opt.GetState().String()],
		),
	}

	switch opt.GetValue().(type) {
	case *envoy_config_core_v3.SocketOption_BufValue:
		downgradedOption.Value = &envoy_api_v2_core.SocketOption_BufValue{
			BufValue: opt.GetBufValue(),
		}
	case *envoy_config_core_v3.SocketOption_IntValue:
		downgradedOption.Value = &envoy_api_v2_core.SocketOption_IntValue{
			IntValue: opt.GetIntValue(),
		}
	}

	return downgradedOption
}

func downgradeMetadata(meta *envoy_config_core_v3.Metadata) *envoy_api_v2_core.Metadata {
	if meta == nil {
		return nil
	}
	return &envoy_api_v2_core.Metadata{
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

func downgradeAddress(address *envoy_config_core_v3.Address) *envoy_api_v2_core.Address {
	if address == nil {
		return nil
	}
	var downgradedAddress *envoy_api_v2_core.Address

	switch typed := address.GetAddress().(type) {
	case *envoy_config_core_v3.Address_SocketAddress:
		downgradedAddress = &envoy_api_v2_core.Address{
			Address: &envoy_api_v2_core.Address_SocketAddress{
				SocketAddress: downgradeSocketAddress(typed.SocketAddress),
			},
		}
	case *envoy_config_core_v3.Address_Pipe:
		downgradedAddress = &envoy_api_v2_core.Address{
			Address: &envoy_api_v2_core.Address_Pipe{
				Pipe: &envoy_api_v2_core.Pipe{
					Path: typed.Pipe.GetPath(),
					Mode: typed.Pipe.GetMode(),
				},
			},
		}
	}

	return downgradedAddress
}

func downgradeSocketAddress(address *envoy_config_core_v3.SocketAddress) *envoy_api_v2_core.SocketAddress {
	if address == nil {
		return nil
	}

	socketAddress := &envoy_api_v2_core.SocketAddress{
		Protocol: envoy_api_v2_core.SocketAddress_Protocol(
			envoy_api_v2_core.SocketAddress_Protocol_value[address.GetProtocol().String()],
		),
		Address:      address.GetAddress(),
		ResolverName: address.GetResolverName(),
		Ipv4Compat:   address.GetIpv4Compat(),
	}
	switch address.GetPortSpecifier().(type) {
	case *envoy_config_core_v3.SocketAddress_PortValue:
		socketAddress.PortSpecifier = &envoy_api_v2_core.SocketAddress_PortValue{
			PortValue: address.GetPortValue(),
		}
	case *envoy_config_core_v3.SocketAddress_NamedPort:
		socketAddress.PortSpecifier = &envoy_api_v2_core.SocketAddress_NamedPort{
			NamedPort: address.GetNamedPort(),
		}
	}
	return socketAddress
}
