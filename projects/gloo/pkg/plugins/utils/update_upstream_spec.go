package utils

import v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

// for use by UDS plugins
// copies parts of the UpstreamSpec that are not
// set by discovery but may be set by the user or function discovery
// so they are not overwritten when UDS resyncs
func UpdateUpstream(original, desired *v1.Upstream) {

	// do not override ssl and subset config if none specified by discovery
	if desired.GetSslConfig() == nil {
		desired.SslConfig = original.SslConfig
	}
	if desired.GetCircuitBreakers() == nil {
		desired.CircuitBreakers = original.CircuitBreakers
	}
	if desired.GetLoadBalancerConfig() == nil {
		desired.LoadBalancerConfig = original.LoadBalancerConfig
	}
	if desired.GetConnectionConfig() == nil {
		desired.ConnectionConfig = original.ConnectionConfig
	}
	if desired.GetFailover() == nil {
		desired.Failover = original.Failover
	}
	if len(desired.GetHealthChecks()) == 0 {
		desired.HealthChecks = original.HealthChecks
	}
	if desired.GetOutlierDetection() == nil {
		desired.OutlierDetection = original.OutlierDetection
	}
	if desired.GetUseHttp2() == nil {
		desired.UseHttp2 = original.UseHttp2
	}

	if desired.GetInitialConnectionWindowSize() == nil {
		desired.InitialConnectionWindowSize = original.InitialConnectionWindowSize
	}

	if desired.GetInitialStreamWindowSize() == nil {
		desired.InitialStreamWindowSize = original.InitialStreamWindowSize
	}

	if desired.GetHttpProxyHostname() == nil {
		desired.HttpProxyHostname = original.HttpProxyHostname
	}

	if desiredSubsetMutator, ok := desired.GetUpstreamType().(v1.SubsetSpecMutator); ok {
		if desiredSubsetMutator.GetSubsetSpec() == nil {
			desiredSubsetMutator.SetSubsetSpec(original.GetUpstreamType().(v1.SubsetSpecGetter).GetSubsetSpec())
		}
	}

}
