package internal

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

func DowngradeCluster(cluster *envoy_config_cluster_v3.Cluster) *envoyapi.Cluster {
	downgradedCluster := &envoyapi.Cluster{
		TransportSocketMatches: make(
			[]*envoyapi.Cluster_TransportSocketMatch, 0, len(cluster.GetTransportSocketMatches()),
		),
		Name:                          cluster.GetName(),
		AltStatName:                   cluster.GetAltStatName(),
		ClusterDiscoveryType:          nil,
		EdsClusterConfig:              downgradeEdsClusterConfig(cluster.GetEdsClusterConfig()),
		ConnectTimeout:                cluster.GetConnectTimeout(),
		PerConnectionBufferLimitBytes: cluster.GetPerConnectionBufferLimitBytes(),
		LbPolicy: envoyapi.Cluster_LbPolicy(
			envoyapi.Cluster_LbPolicy_value[cluster.GetLbPolicy().String()],
		),
		LoadAssignment:                nil,
		HealthChecks:                  nil,
		MaxRequestsPerConnection:      cluster.GetMaxRequestsPerConnection(),
		CircuitBreakers:               nil,
		UpstreamHttpProtocolOptions:   downgradeUpstreamHttpProtocolOptions(cluster.GetUpstreamHttpProtocolOptions()),
		CommonHttpProtocolOptions:     downgradeHttpProtocolOptions(cluster.GetCommonHttpProtocolOptions()),
		HttpProtocolOptions:           downgradeHttp1ProtocolOptions(cluster.GetHttpProtocolOptions()),
		Http2ProtocolOptions:          downgradeHttp2ProtocolOptions(cluster.GetHttp2ProtocolOptions()),
		TypedExtensionProtocolOptions: cluster.GetTypedExtensionProtocolOptions(),
		DnsRefreshRate:                cluster.GetDnsRefreshRate(),
		RespectDnsTtl:                 cluster.GetRespectDnsTtl(),
		DnsLookupFamily: envoyapi.Cluster_DnsLookupFamily(
			envoyapi.Cluster_DnsLookupFamily_value[cluster.GetDnsLookupFamily().String()],
		),
		DnsResolvers:                        make([]*envoy_api_v2_core.Address, 0, len(cluster.GetDnsResolvers())),
		UseTcpForDnsLookups:                 cluster.GetUseTcpForDnsLookups(),
		OutlierDetection:                    downgradeOutlierDetection(cluster.GetOutlierDetection()),
		CleanupInterval:                     cluster.GetCleanupInterval(),
		UpstreamBindConfig:                  nil,
		LbSubsetConfig:                      nil,
		LbConfig:                            nil,
		CommonLbConfig:                      nil,
		TransportSocket:                     downgradeTransportSocket(cluster.GetTransportSocket()),
		Metadata:                            downgradeMetadata(cluster.GetMetadata()),
		ProtocolSelection:                   0,
		UpstreamConnectionOptions:           nil,
		CloseConnectionsOnHostHealthFailure: cluster.GetCloseConnectionsOnHostHealthFailure(),
		Filters:                             make([]*envoy_api_v2_cluster.Filter, 0, len(cluster.GetFilters())),
		LoadBalancingPolicy:                 nil,
		LrsServer:                           downgradeConfigSource(cluster.GetLrsServer()),
		TrackTimeoutBudgets:                 cluster.GetTrackTimeoutBudgets(),
		// Not present in v2
		DrainConnectionsOnHostRemoval: false,
		// Unused and deprecated
		ExtensionProtocolOptions: cluster.GetHiddenEnvoyDeprecatedExtensionProtocolOptions(),
		TlsContext:               nil,
		Hosts:                    nil,
	}

	for _, v := range cluster.GetDnsResolvers() {
		downgradedCluster.DnsResolvers = append(downgradedCluster.DnsResolvers, downgradeAddress(v))
	}

	for _, v := range cluster.GetTransportSocketMatches() {
		downgradedCluster.TransportSocketMatches = append(
			downgradedCluster.TransportSocketMatches, downgradeTransportSocketMatch(v),
		)
	}

	for _, v := range cluster.GetFilters() {
		downgradedCluster.Filters = append(downgradedCluster.Filters, downgradeClusterFilters(v))
	}

	if cluster.GetDnsFailureRefreshRate() != nil {
		downgradedCluster.DnsFailureRefreshRate = &envoyapi.Cluster_RefreshRate{
			BaseInterval: cluster.GetDnsFailureRefreshRate().GetBaseInterval(),
			MaxInterval:  cluster.GetDnsFailureRefreshRate().GetMaxInterval(),
		}
	}
	return downgradedCluster
}

func downgradeBindConfig(cfg *envoy_config_core_v3.BindConfig) *envoy_api_v2_core.BindConfig {
	if cfg == nil {
		return nil
	}

	downgraded := &envoy_api_v2_core.BindConfig{
		Freebind:      cfg.GetFreebind(),
		SourceAddress: downgradeSocketAddress(cfg.GetSourceAddress()),
	}

	return &envoy_api_v2_core.BindConfig{
		SourceAddress: nil,
		Freebind:      nil,
		SocketOptions: nil,
	}
}

func downgradeSocketOption(opt *envoy_config_core_v3.SocketOption) *envoy_api_v2_core.SocketOption {
	if opt == nil {
		return nil
	}

	downgraded := &envoy_api_v2_core.SocketOption{
		Description: opt.GetDescription(),
		Level:       opt.GetLevel(),
		Name:        opt.GetName(),
		State: envoy_api_v2_core.SocketOption_SocketState(
			envoy_api_v2_core.SocketOption_SocketState_value[opt.GetState().String()],
		),
	}

	switch typed := opt.GetValue().(type) {
	case *envoy_config_core_v3.SocketOption_IntValue:

	case *envoy_config_core_v3.SocketOption_BufValue:
	}

	return downgraded

}

func downgradeOutlierDetection(od *envoy_config_cluster_v3.OutlierDetection) *envoy_api_v2_cluster.OutlierDetection {
	if od == nil {
		return nil
	}
	return &envoy_api_v2_cluster.OutlierDetection{
		Consecutive_5Xx:                        od.GetConsecutive_5Xx(),
		Interval:                               od.GetInterval(),
		BaseEjectionTime:                       od.GetBaseEjectionTime(),
		MaxEjectionPercent:                     od.GetMaxEjectionPercent(),
		EnforcingConsecutive_5Xx:               od.GetEnforcingConsecutive_5Xx(),
		EnforcingSuccessRate:                   od.GetEnforcingSuccessRate(),
		SuccessRateMinimumHosts:                od.GetSuccessRateMinimumHosts(),
		SuccessRateRequestVolume:               od.GetSuccessRateRequestVolume(),
		SuccessRateStdevFactor:                 od.GetSuccessRateStdevFactor(),
		ConsecutiveGatewayFailure:              od.GetConsecutiveGatewayFailure(),
		EnforcingConsecutiveGatewayFailure:     od.GetEnforcingConsecutiveGatewayFailure(),
		SplitExternalLocalOriginErrors:         od.GetSplitExternalLocalOriginErrors(),
		ConsecutiveLocalOriginFailure:          od.GetConsecutiveLocalOriginFailure(),
		EnforcingConsecutiveLocalOriginFailure: od.GetEnforcingConsecutiveLocalOriginFailure(),
		EnforcingLocalOriginSuccessRate:        od.GetEnforcingLocalOriginSuccessRate(),
		FailurePercentageThreshold:             od.GetFailurePercentageThreshold(),
		EnforcingFailurePercentage:             od.GetEnforcingFailurePercentage(),
		EnforcingFailurePercentageLocalOrigin:  od.GetEnforcingFailurePercentageLocalOrigin(),
		FailurePercentageMinimumHosts:          od.GetFailurePercentageMinimumHosts(),
		FailurePercentageRequestVolume:         od.GetFailurePercentageRequestVolume(),
	}
}

func downgradeHttpProtocolOptions(
	opt *envoy_config_core_v3.HttpProtocolOptions,
) *envoy_api_v2_core.HttpProtocolOptions {
	if opt == nil {
		return nil
	}

	return &envoy_api_v2_core.HttpProtocolOptions{
		IdleTimeout:           opt.GetIdleTimeout(),
		MaxConnectionDuration: opt.GetMaxConnectionDuration(),
		MaxHeadersCount:       opt.GetMaxHeadersCount(),
		MaxStreamDuration:     opt.GetMaxStreamDuration(),
		HeadersWithUnderscoresAction: envoy_api_v2_core.HttpProtocolOptions_HeadersWithUnderscoresAction(
			envoy_api_v2_core.HttpProtocolOptions_HeadersWithUnderscoresAction_value[opt.GetHeadersWithUnderscoresAction().String()],
		),
	}
}

func downgradeHttp1ProtocolOptions(
	opt *envoy_config_core_v3.Http1ProtocolOptions,
) *envoy_api_v2_core.Http1ProtocolOptions {
	if opt == nil {
		return nil
	}

	return &envoy_api_v2_core.Http1ProtocolOptions{
		AllowAbsoluteUrl:      opt.GetAllowAbsoluteUrl(),
		AcceptHttp_10:         opt.GetAcceptHttp_10(),
		DefaultHostForHttp_10: opt.GetDefaultHostForHttp_10(),
		// Only one option exists
		HeaderKeyFormat: &envoy_api_v2_core.Http1ProtocolOptions_HeaderKeyFormat{
			HeaderFormat: &envoy_api_v2_core.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords_{
				ProperCaseWords: &envoy_api_v2_core.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords{},
			},
		},
		EnableTrailers: opt.GetEnableTrailers(),
	}
}

func downgradeHttp2ProtocolOptions(
	opt *envoy_config_core_v3.Http2ProtocolOptions,
) *envoy_api_v2_core.Http2ProtocolOptions {
	if opt == nil {
		return nil
	}
	downgraded := &envoy_api_v2_core.Http2ProtocolOptions{
		HpackTableSize:                               opt.GetHpackTableSize(),
		MaxConcurrentStreams:                         opt.GetMaxConcurrentStreams(),
		InitialStreamWindowSize:                      opt.GetInitialStreamWindowSize(),
		InitialConnectionWindowSize:                  opt.GetInitialConnectionWindowSize(),
		AllowConnect:                                 opt.GetAllowConnect(),
		AllowMetadata:                                opt.GetAllowMetadata(),
		MaxOutboundFrames:                            opt.GetMaxOutboundFrames(),
		MaxOutboundControlFrames:                     opt.GetMaxOutboundControlFrames(),
		MaxConsecutiveInboundFramesWithEmptyPayload:  opt.GetMaxConsecutiveInboundFramesWithEmptyPayload(),
		MaxInboundPriorityFramesPerStream:            opt.GetMaxInboundPriorityFramesPerStream(),
		MaxInboundWindowUpdateFramesPerDataFrameSent: opt.GetMaxInboundWindowUpdateFramesPerDataFrameSent(),
		StreamErrorOnInvalidHttpMessaging:            opt.GetStreamErrorOnInvalidHttpMessaging(),
	}

	for _, v := range opt.GetCustomSettingsParameters() {
		downgraded.CustomSettingsParameters = append(
			downgraded.CustomSettingsParameters, &envoy_api_v2_core.Http2ProtocolOptions_SettingsParameter{
				Identifier: v.GetIdentifier(),
				Value:      v.GetValue(),
			},
		)
	}

	return downgraded
}

func downgradeUpstreamHttpProtocolOptions(
	opt *envoy_config_core_v3.UpstreamHttpProtocolOptions,
) *envoy_api_v2_core.UpstreamHttpProtocolOptions {
	if opt == nil {
		return nil
	}

	return &envoy_api_v2_core.UpstreamHttpProtocolOptions{
		AutoSni:           opt.GetAutoSni(),
		AutoSanValidation: opt.GetAutoSanValidation(),
	}
}

func downgradeEdsClusterConfig(cfg *envoy_config_cluster_v3.Cluster_EdsClusterConfig) *envoyapi.Cluster_EdsClusterConfig {
	if cfg == nil {
		return nil
	}

	return &envoyapi.Cluster_EdsClusterConfig{
		EdsConfig:   downgradeConfigSource(cfg.GetEdsConfig()),
		ServiceName: cfg.GetServiceName(),
	}
}

func downgradeClusterFilters(filter *envoy_config_cluster_v3.Filter) *envoy_api_v2_cluster.Filter {
	if filter == nil {
		return nil
	}

	return &envoy_api_v2_cluster.Filter{
		Name:        filter.GetName(),
		TypedConfig: filter.GetTypedConfig(),
	}
}

func downgradeTransportSocketMatch(
	match *envoy_config_cluster_v3.Cluster_TransportSocketMatch,
) *envoyapi.Cluster_TransportSocketMatch {
	if match == nil {
		return nil
	}

	return &envoyapi.Cluster_TransportSocketMatch{
		Name:            match.GetName(),
		Match:           match.GetMatch(),
		TransportSocket: downgradeTransportSocket(match.GetTransportSocket()),
	}
}

func downgradeConfigSource(source *envoy_config_core_v3.ConfigSource) *envoy_api_v2_core.ConfigSource {
	if source == nil {
		return nil
	}

	downgraded := &envoy_api_v2_core.ConfigSource{
		ConfigSourceSpecifier: nil,
		InitialFetchTimeout:   source.GetInitialFetchTimeout(),
		ResourceApiVersion: envoy_api_v2_core.ApiVersion(
			envoy_api_v2_core.ApiVersion_value[source.GetResourceApiVersion().String()],
		),
	}

	switch typed := source.GetConfigSourceSpecifier().(type) {
	case *envoy_config_core_v3.ConfigSource_Ads:
		downgraded.ConfigSourceSpecifier = &envoy_api_v2_core.ConfigSource_Ads{
			Ads: &envoy_api_v2_core.AggregatedConfigSource{},
		}
	case *envoy_config_core_v3.ConfigSource_ApiConfigSource:

		apiConfigSource := &envoy_api_v2_core.ApiConfigSource{
			ApiType: envoy_api_v2_core.ApiConfigSource_ApiType(
				envoy_api_v2_core.ApiConfigSource_ApiType_value[typed.ApiConfigSource.GetApiType().String()],
			),
			TransportApiVersion: envoy_api_v2_core.ApiVersion(
				envoy_api_v2_core.ApiVersion_value[typed.ApiConfigSource.GetTransportApiVersion().String()],
			),
			ClusterNames: typed.ApiConfigSource.GetClusterNames(),
			GrpcServices: make(
				[]*envoy_api_v2_core.GrpcService, 0, len(typed.ApiConfigSource.GetGrpcServices()),
			),
			RefreshDelay:              typed.ApiConfigSource.GetRefreshDelay(),
			RequestTimeout:            typed.ApiConfigSource.GetRequestTimeout(),
			RateLimitSettings:         downgradeRateLimitSettings(typed.ApiConfigSource.GetRateLimitSettings()),
			SetNodeOnFirstMessageOnly: typed.ApiConfigSource.GetSetNodeOnFirstMessageOnly(),
		}

		for _, v := range typed.ApiConfigSource.GetGrpcServices() {
			apiConfigSource.GrpcServices = append(apiConfigSource.GrpcServices, downgradeGrpcService(v))
		}

		downgraded.ConfigSourceSpecifier = &envoy_api_v2_core.ConfigSource_ApiConfigSource{
			ApiConfigSource: apiConfigSource,
		}
	case *envoy_config_core_v3.ConfigSource_Path:
		downgraded.ConfigSourceSpecifier = &envoy_api_v2_core.ConfigSource_Path{
			Path: typed.Path,
		}
	case *envoy_config_core_v3.ConfigSource_Self:
		downgraded.ConfigSourceSpecifier = &envoy_api_v2_core.ConfigSource_Self{
			Self: &envoy_api_v2_core.SelfConfigSource{},
		}
	}

	return downgraded
}

func downgradeGrpcService(svc *envoy_config_core_v3.GrpcService) *envoy_api_v2_core.GrpcService {
	if svc == nil {
		return nil
	}

	downgraded := &envoy_api_v2_core.GrpcService{
		Timeout:         svc.GetTimeout(),
		InitialMetadata: make([]*envoy_api_v2_core.HeaderValue, 0, len(svc.GetInitialMetadata())),
	}

	for _, v := range svc.GetInitialMetadata() {
		downgraded.InitialMetadata = append(downgraded.InitialMetadata, downgradeHeaderValue(v))
	}

	switch typed := svc.GetTargetSpecifier().(type) {
	case *envoy_config_core_v3.GrpcService_EnvoyGrpc_:
		downgraded.TargetSpecifier = &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
				ClusterName: typed.EnvoyGrpc.GetClusterName(),
			},
		}
	case *envoy_config_core_v3.GrpcService_GoogleGrpc_: // Currently unsupported by gloo
	}

	return downgraded
}

func downgradeHeaderValue(hv *envoy_config_core_v3.HeaderValue) *envoy_api_v2_core.HeaderValue {
	if hv == nil {
		return nil
	}

	return &envoy_api_v2_core.HeaderValue{
		Key:   hv.GetKey(),
		Value: hv.GetValue(),
	}
}

func downgradeRateLimitSettings(rls *envoy_config_core_v3.RateLimitSettings) *envoy_api_v2_core.RateLimitSettings {
	if rls == nil {
		return nil
	}
	return &envoy_api_v2_core.RateLimitSettings{
		MaxTokens: rls.GetMaxTokens(),
		FillRate:  rls.GetFillRate(),
	}
}
