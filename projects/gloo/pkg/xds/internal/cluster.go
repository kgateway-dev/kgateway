package internal

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	envoy_type_matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

func DowngradeCluster(cluster *envoy_config_cluster_v3.Cluster) *envoyapi.Cluster {
	if cluster == nil {
		return nil
	}

	downgradedCluster := &envoyapi.Cluster{
		TransportSocketMatches: make(
			[]*envoyapi.Cluster_TransportSocketMatch, 0, len(cluster.GetTransportSocketMatches()),
		),
		Name:                          cluster.GetName(),
		AltStatName:                   cluster.GetAltStatName(),
		EdsClusterConfig:              downgradeEdsClusterConfig(cluster.GetEdsClusterConfig()),
		ConnectTimeout:                cluster.GetConnectTimeout(),
		PerConnectionBufferLimitBytes: cluster.GetPerConnectionBufferLimitBytes(),
		LbPolicy: envoyapi.Cluster_LbPolicy(
			envoyapi.Cluster_LbPolicy_value[cluster.GetLbPolicy().String()],
		),
		LoadAssignment:                DowngradeEndpoint(cluster.GetLoadAssignment()),
		MaxRequestsPerConnection:      cluster.GetMaxRequestsPerConnection(),
		CircuitBreakers:               downgradeCircuitBreakers(cluster.GetCircuitBreakers()),
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
		DnsResolvers:        make([]*envoy_api_v2_core.Address, 0, len(cluster.GetDnsResolvers())),
		UseTcpForDnsLookups: cluster.GetUseTcpForDnsLookups(),
		OutlierDetection:    downgradeOutlierDetection(cluster.GetOutlierDetection()),
		CleanupInterval:     cluster.GetCleanupInterval(),
		UpstreamBindConfig:  downgradeBindConfig(cluster.GetUpstreamBindConfig()),
		CommonLbConfig:      downgradeCommonLbConfig(cluster.GetCommonLbConfig()),
		LbSubsetConfig:      downgradeLbSubsetConfig(cluster.GetLbSubsetConfig()),
		TransportSocket:     downgradeTransportSocket(cluster.GetTransportSocket()),
		Metadata:            downgradeMetadata(cluster.GetMetadata()),
		ProtocolSelection: envoyapi.Cluster_ClusterProtocolSelection(
			envoyapi.Cluster_ClusterProtocolSelection_value[cluster.GetProtocolSelection().String()],
		),
		UpstreamConnectionOptions:           downgradeUpstreamConnectionOptions(cluster.GetUpstreamConnectionOptions()),
		CloseConnectionsOnHostHealthFailure: cluster.GetCloseConnectionsOnHostHealthFailure(),
		Filters:                             make([]*envoy_api_v2_cluster.Filter, 0, len(cluster.GetFilters())),
		LoadBalancingPolicy:                 downgradeLoadBalancingPolicy(cluster.GetLoadBalancingPolicy()),
		LrsServer:                           downgradeConfigSource(cluster.GetLrsServer()),
		TrackTimeoutBudgets:                 cluster.GetTrackTimeoutBudgets(),
		// Not present in v2
		DrainConnectionsOnHostRemoval: false,
	}

	switch typed := cluster.GetClusterDiscoveryType().(type) {
	case *envoy_config_cluster_v3.Cluster_Type:
		downgradedCluster.ClusterDiscoveryType = &envoyapi.Cluster_Type{
			Type: envoyapi.Cluster_DiscoveryType(envoyapi.Cluster_DiscoveryType_value[typed.Type.String()]),
		}
	case *envoy_config_cluster_v3.Cluster_ClusterType:
		downgradedCluster.ClusterDiscoveryType = &envoyapi.Cluster_ClusterType{
			ClusterType: &envoyapi.Cluster_CustomClusterType{
				Name:        typed.ClusterType.GetName(),
				TypedConfig: typed.ClusterType.GetTypedConfig(),
			},
		}
	}

	switch typed := cluster.GetLbConfig().(type) {
	case *envoy_config_cluster_v3.Cluster_RingHashLbConfig_:
		downgradedCluster.LbConfig = &envoyapi.Cluster_RingHashLbConfig_{
			RingHashLbConfig: &envoyapi.Cluster_RingHashLbConfig{
				MinimumRingSize: typed.RingHashLbConfig.GetMinimumRingSize(),
				HashFunction: envoyapi.Cluster_RingHashLbConfig_HashFunction(
					envoyapi.Cluster_RingHashLbConfig_HashFunction_value[typed.RingHashLbConfig.GetHashFunction().String()],
				),
				MaximumRingSize: typed.RingHashLbConfig.GetMaximumRingSize(),
			},
		}
	case *envoy_config_cluster_v3.Cluster_OriginalDstLbConfig_:
		downgradedCluster.LbConfig = &envoyapi.Cluster_OriginalDstLbConfig_{
			OriginalDstLbConfig: &envoyapi.Cluster_OriginalDstLbConfig{
				UseHttpHeader: typed.OriginalDstLbConfig.GetUseHttpHeader(),
			},
		}
	case *envoy_config_cluster_v3.Cluster_LeastRequestLbConfig_:
		downgradedCluster.LbConfig = &envoyapi.Cluster_LeastRequestLbConfig_{
			LeastRequestLbConfig: &envoyapi.Cluster_LeastRequestLbConfig{
				ChoiceCount: typed.LeastRequestLbConfig.GetChoiceCount(),
			},
		}
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

	for _, v := range cluster.GetHealthChecks() {
		downgradedCluster.HealthChecks = append(downgradedCluster.HealthChecks, downgradeHealthCheck(v))
	}

	if cluster.GetDnsFailureRefreshRate() != nil {
		downgradedCluster.DnsFailureRefreshRate = &envoyapi.Cluster_RefreshRate{
			BaseInterval: cluster.GetDnsFailureRefreshRate().GetBaseInterval(),
			MaxInterval:  cluster.GetDnsFailureRefreshRate().GetMaxInterval(),
		}
	}
	return downgradedCluster
}

func downgradeLbSubsetConfig(cfg *envoy_config_cluster_v3.Cluster_LbSubsetConfig) *envoyapi.Cluster_LbSubsetConfig {
	if cfg == nil {
		return nil
	}

	downgraded := &envoyapi.Cluster_LbSubsetConfig{
		FallbackPolicy: envoyapi.Cluster_LbSubsetConfig_LbSubsetFallbackPolicy(
			envoyapi.Cluster_LbSubsetConfig_LbSubsetFallbackPolicy_value[cfg.GetFallbackPolicy().String()],
		),
		DefaultSubset:       cfg.GetDefaultSubset(),
		SubsetSelectors:     make([]*envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector, 0, len(cfg.GetSubsetSelectors())),
		LocalityWeightAware: cfg.GetLocalityWeightAware(),
		ScaleLocalityWeight: cfg.GetScaleLocalityWeight(),
		PanicModeAny:        cfg.GetPanicModeAny(),
		ListAsAny:           cfg.GetListAsAny(),
	}

	for _, v := range cfg.GetSubsetSelectors() {
		downgraded.SubsetSelectors = append(downgraded.SubsetSelectors, downgradeLbSubsetSelector(v))
	}

	return downgraded
}

func downgradeLbSubsetSelector(
	sel *envoy_config_cluster_v3.Cluster_LbSubsetConfig_LbSubsetSelector,
) *envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector {
	if sel == nil {
		return nil
	}

	return &envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector{
		Keys: sel.GetKeys(),
		FallbackPolicy: envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector_LbSubsetSelectorFallbackPolicy(
			envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector_LbSubsetSelectorFallbackPolicy_value[sel.GetFallbackPolicy().String()],
		),
		FallbackKeysSubset: sel.GetFallbackKeysSubset(),
	}
}

func downgradeUpstreamConnectionOptions(
	u *envoy_config_cluster_v3.UpstreamConnectionOptions,
) *envoyapi.UpstreamConnectionOptions {
	if u == nil {
		return nil
	}

	return &envoyapi.UpstreamConnectionOptions{
		TcpKeepalive: downgradeTcpKeepalive(u.GetTcpKeepalive()),
	}
}

func downgradeTcpKeepalive(t *envoy_config_core_v3.TcpKeepalive) *envoy_api_v2_core.TcpKeepalive {
	if t == nil {
		return nil
	}

	return &envoy_api_v2_core.TcpKeepalive{
		KeepaliveProbes:   t.GetKeepaliveProbes(),
		KeepaliveTime:     t.GetKeepaliveTime(),
		KeepaliveInterval: t.GetKeepaliveInterval(),
	}
}

func downgradeCommonLbConfig(cfg *envoy_config_cluster_v3.Cluster_CommonLbConfig) *envoyapi.Cluster_CommonLbConfig {
	if cfg == nil {
		return nil
	}

	downgraded := &envoyapi.Cluster_CommonLbConfig{
		HealthyPanicThreshold:           downgradePercent(cfg.GetHealthyPanicThreshold()),
		UpdateMergeWindow:               cfg.GetUpdateMergeWindow(),
		IgnoreNewHostsUntilFirstHc:      cfg.GetIgnoreNewHostsUntilFirstHc(),
		CloseConnectionsOnHostSetChange: cfg.GetCloseConnectionsOnHostSetChange(),
	}

	if cfg.GetConsistentHashingLbConfig() != nil {
		downgraded.ConsistentHashingLbConfig = &envoyapi.Cluster_CommonLbConfig_ConsistentHashingLbConfig{
			UseHostnameForHashing: cfg.GetConsistentHashingLbConfig().GetUseHostnameForHashing(),
		}
	}

	switch typed := cfg.GetLocalityConfigSpecifier().(type) {
	case *envoy_config_cluster_v3.Cluster_CommonLbConfig_ZoneAwareLbConfig_:
		downgraded.LocalityConfigSpecifier = &envoyapi.Cluster_CommonLbConfig_ZoneAwareLbConfig_{
			ZoneAwareLbConfig: &envoyapi.Cluster_CommonLbConfig_ZoneAwareLbConfig{
				RoutingEnabled:     downgradePercent(typed.ZoneAwareLbConfig.GetRoutingEnabled()),
				MinClusterSize:     typed.ZoneAwareLbConfig.GetMinClusterSize(),
				FailTrafficOnPanic: typed.ZoneAwareLbConfig.GetFailTrafficOnPanic(),
			},
		}
	case *envoy_config_cluster_v3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_:
		downgraded.LocalityConfigSpecifier = &envoyapi.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
			LocalityWeightedLbConfig: &envoyapi.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
		}
	}

	return downgraded
}

func downgradeCircuitBreakers(cb *envoy_config_cluster_v3.CircuitBreakers) *envoy_api_v2_cluster.CircuitBreakers {
	if cb == nil {
		return nil
	}

	downgraded := &envoy_api_v2_cluster.CircuitBreakers{
		Thresholds: make([]*envoy_api_v2_cluster.CircuitBreakers_Thresholds, 0, len(cb.GetThresholds())),
	}

	for _, v := range cb.GetThresholds() {
		downgraded.Thresholds = append(downgraded.Thresholds, downgradeCircuitBreakerThreshold(v))
	}

	return downgraded
}

func downgradeCircuitBreakerThreshold(
	t *envoy_config_cluster_v3.CircuitBreakers_Thresholds,
) *envoy_api_v2_cluster.CircuitBreakers_Thresholds {
	if t == nil {
		return nil
	}

	return &envoy_api_v2_cluster.CircuitBreakers_Thresholds{
		Priority: envoy_api_v2_core.RoutingPriority(
			envoy_api_v2_core.RoutingPriority_value[t.GetPriority().String()],
		),
		MaxConnections:     t.GetMaxConnections(),
		MaxPendingRequests: t.GetMaxPendingRequests(),
		MaxRequests:        t.GetMaxRequests(),
		MaxRetries:         t.GetMaxRetries(),
		RetryBudget:        downgradeRetryBudget(t.GetRetryBudget()),
		TrackRemaining:     t.GetTrackRemaining(),
		MaxConnectionPools: t.GetMaxConnectionPools(),
	}
}

func downgradeRetryBudget(
	b *envoy_config_cluster_v3.CircuitBreakers_Thresholds_RetryBudget,
) *envoy_api_v2_cluster.CircuitBreakers_Thresholds_RetryBudget {
	if b == nil {
		return nil
	}

	return &envoy_api_v2_cluster.CircuitBreakers_Thresholds_RetryBudget{
		BudgetPercent:       downgradePercent(b.GetBudgetPercent()),
		MinRetryConcurrency: b.GetMinRetryConcurrency(),
	}
}

func downgradeHealthCheck(hc *envoy_config_core_v3.HealthCheck) *envoy_api_v2_core.HealthCheck {
	if hc == nil {
		return nil
	}

	downgraded := &envoy_api_v2_core.HealthCheck{
		Timeout:                      hc.GetTimeout(),
		Interval:                     hc.GetInterval(),
		InitialJitter:                hc.GetInitialJitter(),
		IntervalJitter:               hc.GetIntervalJitter(),
		IntervalJitterPercent:        hc.GetIntervalJitterPercent(),
		UnhealthyThreshold:           hc.GetUnhealthyThreshold(),
		HealthyThreshold:             hc.GetHealthyThreshold(),
		AltPort:                      hc.GetAltPort(),
		ReuseConnection:              hc.GetReuseConnection(),
		NoTrafficInterval:            hc.GetNoTrafficInterval(),
		UnhealthyInterval:            hc.GetUnhealthyInterval(),
		UnhealthyEdgeInterval:        hc.GetUnhealthyEdgeInterval(),
		HealthyEdgeInterval:          hc.GetHealthyEdgeInterval(),
		EventLogPath:                 hc.GetEventLogPath(),
		TlsOptions:                   downgradeHealthCheckTlsOptions(hc.GetTlsOptions()),
		AlwaysLogHealthCheckFailures: hc.GetAlwaysLogHealthCheckFailures(),
		// Unused By Gloo
		// EventService:                 hc.GetEventService(),
	}

	switch hc.GetHealthChecker().(type) {
	case *envoy_config_core_v3.HealthCheck_HttpHealthCheck_:
		downgraded.HealthChecker = &envoy_api_v2_core.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: downgradeHttpHealthCheck(hc.GetHttpHealthCheck()),
		}
	case *envoy_config_core_v3.HealthCheck_TcpHealthCheck_:
		downgraded.HealthChecker = &envoy_api_v2_core.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: downgradeTcpHealthCheck(hc.GetTcpHealthCheck()),
		}
	case *envoy_config_core_v3.HealthCheck_GrpcHealthCheck_:
		downgraded.HealthChecker = &envoy_api_v2_core.HealthCheck_GrpcHealthCheck_{
			GrpcHealthCheck: downgradeGrpcHealthCheck(hc.GetGrpcHealthCheck()),
		}
	case *envoy_config_core_v3.HealthCheck_CustomHealthCheck_:
		downgraded.HealthChecker = &envoy_api_v2_core.HealthCheck_CustomHealthCheck_{
			CustomHealthCheck: &envoy_api_v2_core.HealthCheck_CustomHealthCheck{
				Name: hc.GetCustomHealthCheck().GetName(),
				ConfigType: &envoy_api_v2_core.HealthCheck_CustomHealthCheck_TypedConfig{
					TypedConfig: hc.GetCustomHealthCheck().GetTypedConfig(),
				},
			},
		}
	}

	return downgraded
}

func downgradeGrpcHealthCheck(
	hc *envoy_config_core_v3.HealthCheck_GrpcHealthCheck,
) *envoy_api_v2_core.HealthCheck_GrpcHealthCheck {
	if hc == nil {
		return nil
	}

	return &envoy_api_v2_core.HealthCheck_GrpcHealthCheck{
		ServiceName: hc.GetServiceName(),
		Authority:   hc.GetAuthority(),
	}
}

func downgradeTcpHealthCheck(
	hc *envoy_config_core_v3.HealthCheck_TcpHealthCheck,
) *envoy_api_v2_core.HealthCheck_TcpHealthCheck {
	if hc == nil {
		return nil
	}

	downgraded := &envoy_api_v2_core.HealthCheck_TcpHealthCheck{
		Send:    downgradeHealthCheckPayload(hc.GetSend()),
		Receive: make([]*envoy_api_v2_core.HealthCheck_Payload, 0, len(hc.GetReceive())),
	}

	for _, v := range hc.GetReceive() {
		downgraded.Receive = append(downgraded.Receive, downgradeHealthCheckPayload(v))
	}

	return downgraded
}

func downgradeHttpHealthCheck(
	hc *envoy_config_core_v3.HealthCheck_HttpHealthCheck,
) *envoy_api_v2_core.HealthCheck_HttpHealthCheck {
	if hc == nil {
		return nil
	}

	downgraded := &envoy_api_v2_core.HealthCheck_HttpHealthCheck{
		Host:                   hc.GetHost(),
		Path:                   hc.GetPath(),
		Send:                   downgradeHealthCheckPayload(hc.GetSend()),
		Receive:                downgradeHealthCheckPayload(hc.GetReceive()),
		RequestHeadersToAdd:    make([]*envoy_api_v2_core.HeaderValueOption, 0, len(hc.GetRequestHeadersToAdd())),
		RequestHeadersToRemove: hc.GetRequestHeadersToRemove(),
		ExpectedStatuses:       make([]*envoy_type.Int64Range, 0, len(hc.GetExpectedStatuses())),
		CodecClientType: envoy_type.CodecClientType(
			envoy_type.CodecClientType_value[hc.GetCodecClientType().String()],
		),
		ServiceNameMatcher: downgradeStringMatcher(hc.GetServiceNameMatcher()),
	}

	for _, v := range hc.GetExpectedStatuses() {
		downgraded.ExpectedStatuses = append(downgraded.ExpectedStatuses, downgradeInt64Range(v))
	}

	for _, v := range hc.GetRequestHeadersToAdd() {
		downgraded.RequestHeadersToAdd = append(downgraded.RequestHeadersToAdd, downgradeHeaderValueOption(v))
	}

	return downgraded
}

func downgradeHeaderValueOption(opt *envoy_config_core_v3.HeaderValueOption) *envoy_api_v2_core.HeaderValueOption {
	if opt == nil {
		return nil
	}

	return &envoy_api_v2_core.HeaderValueOption{
		Header: downgradeHeaderValue(opt.GetHeader()),
		Append: opt.GetAppend(),
	}
}

func downgradeHealthCheckPayload(
	pl *envoy_config_core_v3.HealthCheck_Payload,
) (downgraded *envoy_api_v2_core.HealthCheck_Payload) {
	if pl == nil {
		return
	}

	switch pl.GetPayload().(type) {
	case *envoy_config_core_v3.HealthCheck_Payload_Text:
		downgraded = &envoy_api_v2_core.HealthCheck_Payload{
			Payload: &envoy_api_v2_core.HealthCheck_Payload_Text{
				Text: pl.GetText(),
			},
		}
	case *envoy_config_core_v3.HealthCheck_Payload_Binary:
		downgraded = &envoy_api_v2_core.HealthCheck_Payload{
			Payload: &envoy_api_v2_core.HealthCheck_Payload_Binary{
				Binary: pl.GetBinary(),
			},
		}
	}

	return
}

func downgradeInt64Range(rng *envoy_type_v3.Int64Range) *envoy_type.Int64Range {
	if rng == nil {
		return nil
	}
	return &envoy_type.Int64Range{
		Start: rng.GetStart(),
		End:   rng.GetEnd(),
	}
}

func downgradeStringMatcher(sm *envoy_type_matcher_v3.StringMatcher) *envoy_type_matcher.StringMatcher {
	if sm == nil {
		return nil
	}

	downgraded := &envoy_type_matcher.StringMatcher{
		IgnoreCase: sm.GetIgnoreCase(),
	}

	switch typed := sm.GetMatchPattern().(type) {
	case *envoy_type_matcher_v3.StringMatcher_Exact:
		downgraded.MatchPattern = &envoy_type_matcher.StringMatcher_Exact{
			Exact: typed.Exact,
		}
	case *envoy_type_matcher_v3.StringMatcher_Prefix:
		downgraded.MatchPattern = &envoy_type_matcher.StringMatcher_Prefix{
			Prefix: typed.Prefix,
		}
	case *envoy_type_matcher_v3.StringMatcher_SafeRegex:
		downgraded.MatchPattern = &envoy_type_matcher.StringMatcher_SafeRegex{
			SafeRegex: &envoy_type_matcher.RegexMatcher{
				EngineType: &envoy_type_matcher.RegexMatcher_GoogleRe2{
					GoogleRe2: &envoy_type_matcher.RegexMatcher_GoogleRE2{
						MaxProgramSize: typed.SafeRegex.GetGoogleRe2().GetMaxProgramSize(),
					},
				},
				Regex: typed.SafeRegex.GetRegex(),
			},
		}
	case *envoy_type_matcher_v3.StringMatcher_Suffix:
		downgraded.MatchPattern = &envoy_type_matcher.StringMatcher_Suffix{
			Suffix: typed.Suffix,
		}
	}

	return downgraded
}

func downgradeHealthCheckTlsOptions(
	opt *envoy_config_core_v3.HealthCheck_TlsOptions,
) *envoy_api_v2_core.HealthCheck_TlsOptions {
	if opt == nil {
		return nil
	}

	return &envoy_api_v2_core.HealthCheck_TlsOptions{
		AlpnProtocols: opt.GetAlpnProtocols(),
	}
}

func downgradeLoadBalancingPolicy(
	policy *envoy_config_cluster_v3.LoadBalancingPolicy,
) *envoyapi.LoadBalancingPolicy {
	if policy == nil {
		return nil
	}

	downgraded := &envoyapi.LoadBalancingPolicy{
		Policies: make([]*envoyapi.LoadBalancingPolicy_Policy, 0, len(policy.GetPolicies())),
	}

	for _, v := range policy.GetPolicies() {
		downgraded.Policies = append(downgraded.Policies, &envoyapi.LoadBalancingPolicy_Policy{
			Name:        v.GetName(),
			TypedConfig: v.GetTypedConfig(),
		})
	}

	return downgraded
}

func downgradeBindConfig(cfg *envoy_config_core_v3.BindConfig) *envoy_api_v2_core.BindConfig {
	if cfg == nil {
		return nil
	}

	downgraded := &envoy_api_v2_core.BindConfig{
		Freebind:      cfg.GetFreebind(),
		SourceAddress: downgradeSocketAddress(cfg.GetSourceAddress()),
		SocketOptions: make([]*envoy_api_v2_core.SocketOption, 0, len(cfg.GetSocketOptions())),
	}

	for _, v := range cfg.GetSocketOptions() {
		downgraded.SocketOptions = append(downgraded.SocketOptions, downgradeSocketOption(v))
	}

	return &envoy_api_v2_core.BindConfig{
		SourceAddress: nil,
		Freebind:      nil,
		SocketOptions: nil,
	}
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
