package utils

import v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

// for use by UDS plugins
// copies parts of the UpstreamSpec that are not
// set by discovery but may be set by the user or function discovery
// so they are not overwritten when UDS resyncs
func UpdateUpstream(original, desired *v1.Upstream) {

	// do not override ssl and subset config if none specified by discovery
	if desired.SslConfig == nil {
		desired.SslConfig = original.SslConfig
	}
	if desired.CircuitBreakers == nil {
		desired.CircuitBreakers = original.CircuitBreakers
	}
	if desired.LoadBalancerConfig == nil {
		desired.LoadBalancerConfig = original.LoadBalancerConfig
	}
	if desired.ConnectionConfig == nil {
		desired.ConnectionConfig = original.ConnectionConfig
	}
	if desired.Failover == nil {
		desired.Failover = original.Failover
	}
	if len(desired.HealthChecks) == 0 {
		desired.HealthChecks = original.HealthChecks
	}
	if desired.OutlierDetection == nil {
		desired.OutlierDetection = original.OutlierDetection
	}
	if desired.UseHttp2 == nil {
		desired.UseHttp2 = original.UseHttp2
	}

	if desired.InitialConnectionWindowSize == nil {
		desired.InitialConnectionWindowSize = original.InitialConnectionWindowSize
	}

	if desired.InitialStreamWindowSize == nil {
		desired.InitialStreamWindowSize = original.InitialStreamWindowSize
	}

	if desired.HttpProxyHostname == nil {
		desired.HttpProxyHostname = original.HttpProxyHostname
	}

	if desiredSubsetMutator, ok := desired.UpstreamType.(v1.SubsetSpecMutator); ok {
		if desiredSubsetMutator.GetSubsetSpec() == nil {
			desiredSubsetMutator.SetSubsetSpec(original.UpstreamType.(v1.SubsetSpecGetter).GetSubsetSpec())
		}
	}

}
