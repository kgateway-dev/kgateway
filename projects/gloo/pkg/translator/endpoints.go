package translator

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"go.opencensus.io/trace"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

const EnvoyLb = "envoy.lb"
const SoloAnnotations = "io.solo.annotations"

// Endpoints

func (t *translatorInstance) computeClusterEndpoints(
	params plugins.Params,
	upstreamRefKeyToEndpoints map[string][]*v1.Endpoint,
	reports reporter.ResourceReports,
) []*envoy_config_endpoint_v3.ClusterLoadAssignment {

	_, span := trace.StartSpan(params.Ctx, "gloo.translator.computeClusterEndpoints")
	defer span.End()

	var clusterEndpointAssignments []*envoy_config_endpoint_v3.ClusterLoadAssignment
	for _, upstream := range params.Snapshot.Upstreams {
		clusterEndpoints := upstreamRefKeyToEndpoints[upstream.GetMetadata().Ref().Key()]
		// if there are any endpoints for this upstream, it's using eds and we need to create a load assignment for it
		if len(clusterEndpoints) > 0 {
			loadAssignment := loadAssignmentForUpstream(upstream, clusterEndpoints)
			for _, plugin := range t.pluginRegistry.GetEndpointPlugins() {
				if err := plugin.ProcessEndpoints(params, upstream, loadAssignment); err != nil {
					reports.AddError(upstream, err)
				}
			}
			clusterEndpointAssignments = append(clusterEndpointAssignments, loadAssignment)
		}
	}
	return clusterEndpointAssignments
}

func loadAssignmentForUpstream(
	upstream *v1.Upstream,
	clusterEndpoints []*v1.Endpoint,
) *envoy_config_endpoint_v3.ClusterLoadAssignment {
	clusterName := upstreamToClusterName(upstream)
	var endpoints []*envoy_config_endpoint_v3.LbEndpoint
	for _, addr := range clusterEndpoints {
		metadata := getLbMetadata(upstream, addr.GetMetadata().GetLabels(), "")
		metadata = addAnnotations(metadata, addr.GetMetadata().GetAnnotations())
		var healthCheckConfig *envoy_config_endpoint_v3.Endpoint_HealthCheckConfig
		if host := addr.GetHealthCheck().GetHostname(); host != "" {
			healthCheckConfig = &envoy_config_endpoint_v3.Endpoint_HealthCheckConfig{
				Hostname: host,
			}
		}
		lbEndpoint := envoy_config_endpoint_v3.LbEndpoint{
			Metadata: metadata,
			HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
				Endpoint: &envoy_config_endpoint_v3.Endpoint{
					Address: &envoy_config_core_v3.Address{
						Address: &envoy_config_core_v3.Address_SocketAddress{
							SocketAddress: &envoy_config_core_v3.SocketAddress{
								Protocol: envoy_config_core_v3.SocketAddress_TCP,
								Address:  addr.GetAddress(),
								PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
									PortValue: addr.GetPort(),
								},
							},
						},
					},
					HealthCheckConfig: healthCheckConfig,
					Hostname:          addr.GetHostname(),
				},
			},
		}
		endpoints = append(endpoints, &lbEndpoint)
	}

	return &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: clusterName,
		Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{{
			LbEndpoints: endpoints,
		}},
	}
}

func createUpstreamToEndpointsMap(upstreams []*v1.Upstream, endpoints []*v1.Endpoint) map[string][]*v1.Endpoint {
	upstreamRefKeyToEndpoints := map[string][]*v1.Endpoint{}
	for _, us := range upstreams {
		var eps []*v1.Endpoint
		upstreamRefKeyToEndpoints[us.GetMetadata().Ref().Key()] = eps
	}
	for _, ep := range endpoints {
		for _, upstreamRef := range ep.GetUpstreams() {
			if eps, ok := upstreamRefKeyToEndpoints[upstreamRef.Key()]; ok {
				eps = append(eps, ep)
				upstreamRefKeyToEndpoints[upstreamRef.Key()] = eps
			}
		}
	}
	return upstreamRefKeyToEndpoints
}

func addAnnotations(metadata *envoy_config_core_v3.Metadata, annotations map[string]string) *envoy_config_core_v3.Metadata {
	if annotations == nil {
		return metadata
	}
	if metadata == nil {
		metadata = &envoy_config_core_v3.Metadata{
			FilterMetadata: map[string]*structpb.Struct{},
		}
	}

	if metadata.GetFilterMetadata() == nil {
		metadata.FilterMetadata = map[string]*structpb.Struct{}
	}

	fields := map[string]*structpb.Value{}
	for k, v := range annotations {
		fields[k] = &structpb.Value{
			Kind: &structpb.Value_StringValue{
				StringValue: v,
			},
		}
	}

	metadata.GetFilterMetadata()[SoloAnnotations] = &structpb.Struct{
		Fields: fields,
	}
	return metadata
}

func getLbMetadata(upstream *v1.Upstream, labels map[string]string, zeroValue string) *envoy_config_core_v3.Metadata {

	meta := &envoy_config_core_v3.Metadata{
		FilterMetadata: map[string]*structpb.Struct{},
	}

	labelsStruct := &structpb.Struct{
		Fields: map[string]*structpb.Value{},
	}

	if upstream != nil {
		for _, k := range allKeys(upstream) {
			labelsStruct.GetFields()[k] = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: zeroValue,
				},
			}
		}
	}

	if labels != nil {
		for k, v := range labels {
			labelsStruct.GetFields()[k] = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: v,
				},
			}
		}
	}

	if len(labelsStruct.GetFields()) == 0 {
		return nil
	}

	meta.GetFilterMetadata()[EnvoyLb] = labelsStruct
	return meta
}

func allKeys(upstream *v1.Upstream) []string {
	specGetter, ok := upstream.GetUpstreamType().(v1.SubsetSpecGetter)
	if !ok {
		return nil
	}
	glooSubsetConfig := specGetter.GetSubsetSpec()
	if glooSubsetConfig == nil {
		return nil
	}
	keysSet := map[string]bool{}

	for _, keys := range glooSubsetConfig.GetSelectors() {
		for _, key := range keys.GetKeys() {
			keysSet[key] = true
		}
	}

	var allKeys []string
	for k := range keysSet {
		allKeys = append(allKeys, k)
	}
	return allKeys
}
