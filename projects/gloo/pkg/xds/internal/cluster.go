package internal

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
)

func DowngradeCluster(cluster *envoy_config_cluster_v3.Cluster) *envoyapi.Cluster {
	downgradedCluster := &envoyapi.Cluster{
		TransportSocketMatches:              nil,
		Name:                                "",
		AltStatName:                         "",
		ClusterDiscoveryType:                nil,
		EdsClusterConfig:                    nil,
		ConnectTimeout:                      nil,
		PerConnectionBufferLimitBytes:       nil,
		LbPolicy:                            0,
		Hosts:                               nil,
		LoadAssignment:                      nil,
		HealthChecks:                        nil,
		MaxRequestsPerConnection:            nil,
		CircuitBreakers:                     nil,
		TlsContext:                          nil,
		UpstreamHttpProtocolOptions:         nil,
		CommonHttpProtocolOptions:           nil,
		HttpProtocolOptions:                 nil,
		Http2ProtocolOptions:                nil,
		ExtensionProtocolOptions:            nil,
		TypedExtensionProtocolOptions:       nil,
		DnsRefreshRate:                      nil,
		DnsFailureRefreshRate:               nil,
		RespectDnsTtl:                       false,
		DnsLookupFamily:                     0,
		DnsResolvers:                        nil,
		UseTcpForDnsLookups:                 false,
		OutlierDetection:                    nil,
		CleanupInterval:                     nil,
		UpstreamBindConfig:                  nil,
		LbSubsetConfig:                      nil,
		LbConfig:                            nil,
		CommonLbConfig:                      nil,
		TransportSocket:                     nil,
		Metadata:                            nil,
		ProtocolSelection:                   0,
		UpstreamConnectionOptions:           nil,
		CloseConnectionsOnHostHealthFailure: false,
		DrainConnectionsOnHostRemoval:       false,
		Filters:                             nil,
		LoadBalancingPolicy:                 nil,
		LrsServer:                           nil,
		TrackTimeoutBudgets:                 false,
		XXX_NoUnkeyedLiteral:                struct{}{},
		XXX_unrecognized:                    nil,
		XXX_sizecache:                       0,
	}
}