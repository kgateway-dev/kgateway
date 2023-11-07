package utils

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// UpdateUpstream is used by UDS plugins
// copies parts of the UpstreamSpec that are not
// set by discovery but may be set by the user or function discovery.
// This way they are not overwritten when UDS resyncs
func UpdateUpstream(original, desired *v1.Upstream) {

	// do not override ssl and subset config if none specified by discovery
	if desired.GetSslConfig() == nil {
		desired.SslConfig = original.GetSslConfig()
	}

	if desired.GetHttpConnectSslConfig() == nil {
		desired.HttpConnectSslConfig = original.GetHttpConnectSslConfig()
	}

	if desired.GetCircuitBreakers() == nil {
		desired.CircuitBreakers = original.GetCircuitBreakers()
	}

	if desired.GetLoadBalancerConfig() == nil {
		desired.LoadBalancerConfig = original.GetLoadBalancerConfig()
	}

	if desired.GetConnectionConfig() == nil {
		desired.ConnectionConfig = original.GetConnectionConfig()
	}

	if desired.GetFailover() == nil {
		desired.Failover = original.GetFailover()
	}

	if len(desired.GetHealthChecks()) == 0 {
		desired.HealthChecks = original.GetHealthChecks()
	}

	if desired.GetOutlierDetection() == nil {
		desired.OutlierDetection = original.GetOutlierDetection()
	}

	if desired.GetUseHttp2() == nil {
		desired.UseHttp2 = original.GetUseHttp2()
	}

	if desired.GetInitialConnectionWindowSize() == nil {
		desired.InitialConnectionWindowSize = original.GetInitialConnectionWindowSize()
	}

	if desired.GetInitialStreamWindowSize() == nil {
		desired.InitialStreamWindowSize = original.GetInitialStreamWindowSize()
	}

	if desired.GetOverrideStreamErrorOnInvalidHttpMessage() == nil {
		desired.OverrideStreamErrorOnInvalidHttpMessage = original.GetOverrideStreamErrorOnInvalidHttpMessage()
	}

	if desired.GetHttpProxyHostname() == nil {
		desired.HttpProxyHostname = original.GetHttpProxyHostname()
	}

	if desiredSubsetMutator, ok := desired.GetUpstreamType().(v1.SubsetSpecMutator); ok {
		if desiredSubsetMutator.GetSubsetSpec() == nil {
			desiredSubsetMutator.SetSubsetSpec(original.GetUpstreamType().(v1.SubsetSpecGetter).GetSubsetSpec())
		}
	}

	if desired.GetIgnoreHealthOnHostRemoval() == nil {
		desired.IgnoreHealthOnHostRemoval = original.GetIgnoreHealthOnHostRemoval()
	}

	if desired.GetRespectDnsTtl() == nil {
		desired.RespectDnsTtl = original.GetRespectDnsTtl()
	}

	if desired.GetDnsRefreshRate() == nil {
		desired.DnsRefreshRate = original.GetDnsRefreshRate()
	}

	if desired.GetProxyProtocolVersion() == nil {
		desired.ProxyProtocolVersion = original.GetProxyProtocolVersion()
	}

	if desired.GetPreconnectPolicy() == nil {
		desired.PreconnectPolicy = original.GetPreconnectPolicy()
	}
}
