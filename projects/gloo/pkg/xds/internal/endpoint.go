package internal

import (
	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

func DowngradeEndpoint(cla *envoy_config_endpoint_v3.ClusterLoadAssignment) *envoy_api_v2.ClusterLoadAssignment {
	if cla == nil {
		return nil
	}

	downgraded := &envoy_api_v2.ClusterLoadAssignment{
		ClusterName:    cla.GetClusterName(),
		Endpoints:      make([]*envoy_api_v2_endpoint.LocalityLbEndpoints, 0, len(cla.GetEndpoints())),
		NamedEndpoints: nil,
		Policy:         downgradePolicy(cla.GetPolicy()),
	}

	if cla.GetNamedEndpoints() != nil {
		downgraded.NamedEndpoints = make(map[string]*envoy_api_v2_endpoint.Endpoint)
	}
	for name, v := range cla.GetNamedEndpoints() {
		downgraded.NamedEndpoints[name] = downgradeEndpoint(v)
	}

	for _, v := range cla.GetEndpoints() {
		downgraded.Endpoints = append(downgraded.Endpoints, downgradeLocalityLbEndpoint(v))
	}

	return downgraded
}

func downgradeLocalityLbEndpoint(
	endpoints *envoy_config_endpoint_v3.LocalityLbEndpoints,
) *envoy_api_v2_endpoint.LocalityLbEndpoints {
	if endpoints == nil {
		return nil
	}

	downgraded := &envoy_api_v2_endpoint.LocalityLbEndpoints{
		Locality:            downgradeLocality(endpoints.GetLocality()),
		LbEndpoints:         make([]*envoy_api_v2_endpoint.LbEndpoint, 0, len(endpoints.GetLbEndpoints())),
		LoadBalancingWeight: endpoints.GetLoadBalancingWeight(),
		Priority:            endpoints.GetPriority(),
		Proximity:           endpoints.GetProximity(),
	}

	for _, v := range endpoints.GetLbEndpoints() {
		downgraded.LbEndpoints = append(downgraded.LbEndpoints, downgradeLbEndpoint(v))
	}

	return downgraded
}

func downgradeLbEndpoint(ep *envoy_config_endpoint_v3.LbEndpoint) *envoy_api_v2_endpoint.LbEndpoint {
	if ep == nil {
		return nil
	}

	downgraded := &envoy_api_v2_endpoint.LbEndpoint{
		HealthStatus: envoy_api_v2_core.HealthStatus(
			envoy_api_v2_core.HealthStatus_value[ep.GetHealthStatus().String()],
		),
		Metadata:            downgradeMetadata(ep.GetMetadata()),
		LoadBalancingWeight: ep.GetLoadBalancingWeight(),
	}

	switch ep.GetHostIdentifier().(type) {
	case *envoy_config_endpoint_v3.LbEndpoint_Endpoint:
		downgraded.HostIdentifier = &envoy_api_v2_endpoint.LbEndpoint_Endpoint{
			Endpoint: downgradeEndpoint(ep.GetEndpoint()),
		}
	case *envoy_config_endpoint_v3.LbEndpoint_EndpointName:
		downgraded.HostIdentifier = &envoy_api_v2_endpoint.LbEndpoint_EndpointName{
			EndpointName: ep.GetEndpointName(),
		}
	}

	return downgraded

}

func downgradeLocality(locality *envoy_config_core_v3.Locality) *envoy_api_v2_core.Locality {
	if locality == nil {
		return nil
	}

	return &envoy_api_v2_core.Locality{
		Region:  locality.GetRegion(),
		Zone:    locality.GetZone(),
		SubZone: locality.GetSubZone(),
	}
}

func downgradeEndpoint(ep *envoy_config_endpoint_v3.Endpoint) *envoy_api_v2_endpoint.Endpoint {
	if ep == nil {
		return nil
	}
	return &envoy_api_v2_endpoint.Endpoint{
		Address:           downgradeAddress(ep.GetAddress()),
		HealthCheckConfig: downgradeEndpointHealthCheck(ep.GetHealthCheckConfig()),
		Hostname:          ep.GetHostname(),
	}
}

func downgradeEndpointHealthCheck(
	hc *envoy_config_endpoint_v3.Endpoint_HealthCheckConfig,
) *envoy_api_v2_endpoint.Endpoint_HealthCheckConfig {
	if hc == nil {
		return nil
	}

	return &envoy_api_v2_endpoint.Endpoint_HealthCheckConfig{
		PortValue: hc.GetPortValue(),
		Hostname:  hc.GetHostname(),
	}
}

func downgradePolicy(
	policy *envoy_config_endpoint_v3.ClusterLoadAssignment_Policy,
) *envoy_api_v2.ClusterLoadAssignment_Policy {
	if policy == nil {
		return nil
	}

	downgraded := &envoy_api_v2.ClusterLoadAssignment_Policy{
		DropOverloads: make(
			[]*envoy_api_v2.ClusterLoadAssignment_Policy_DropOverload, 0, len(policy.GetDropOverloads()),
		),
		OverprovisioningFactor: policy.GetOverprovisioningFactor(),
		EndpointStaleAfter:     policy.GetEndpointStaleAfter(),
	}

	for _, v := range policy.GetDropOverloads() {
		downgraded.DropOverloads = append(downgraded.DropOverloads, downgradeDropOverloads(v))
	}

	return downgraded
}

func downgradeDropOverloads(
	overload *envoy_config_endpoint_v3.ClusterLoadAssignment_Policy_DropOverload,
) *envoy_api_v2.ClusterLoadAssignment_Policy_DropOverload {
	if overload == nil {
		return nil
	}

	return &envoy_api_v2.ClusterLoadAssignment_Policy_DropOverload{
		Category:       overload.GetCategory(),
		DropPercentage: downgradeFractionalPercent(overload.GetDropPercentage()),
	}
}

func downgradeFractionalPercent(pct *envoy_type_v3.FractionalPercent) *envoy_type.FractionalPercent {
	if pct == nil {
		return nil
	}

	return &envoy_type.FractionalPercent{
		Numerator: pct.GetNumerator(),
		Denominator: envoy_type.FractionalPercent_DenominatorType(
			envoy_type.FractionalPercent_DenominatorType_value[pct.GetDenominator().String()],
		),
	}
}

func downgradePercent(pct *envoy_type_v3.Percent) *envoy_type.Percent {
	if pct == nil {
		return nil
	}

	return &envoy_type.Percent{
		Value: pct.GetValue(),
	}
}
