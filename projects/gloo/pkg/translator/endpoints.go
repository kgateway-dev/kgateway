package translator

import (
	"context"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"go.opencensus.io/trace"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyendpoints "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

const EnvoyLb = "envoy.lb"
const SoloAnnotations = "io.solo.annotations"

// Endpoints

func computeClusterEndpoints(ctx context.Context, upstreams []*v1.Upstream, endpoints []*v1.Endpoint) []*envoyapi.ClusterLoadAssignment {

	_, span := trace.StartSpan(ctx, "gloo.translator.computeClusterEndpoints")
	defer span.End()

	var clusterEndpointAssignments []*envoyapi.ClusterLoadAssignment
	for _, upstream := range upstreams {
		clusterEndpoints := endpointsForUpstream(upstream, endpoints)
		// if there are any endpoints for this upstream, it's using eds and we need to create a load assignment for it
		if len(clusterEndpoints) > 0 {
			loadAssignment := loadAssignmentForUpstream(upstream, clusterEndpoints)
			clusterEndpointAssignments = append(clusterEndpointAssignments, loadAssignment)
		}
	}
	return clusterEndpointAssignments
}

func loadAssignmentForUpstream(upstream *v1.Upstream, clusterEndpoints []*v1.Endpoint) *envoyapi.ClusterLoadAssignment {
	clusterName := UpstreamToClusterName(upstream.Metadata.Ref())
	var endpoints []*envoyendpoints.LbEndpoint
	for _, addr := range clusterEndpoints {
		metadata := getLbMetadata(upstream, addr.Metadata.Labels, "")
		metadata = addAnnotations(metadata, addr.Metadata.Annotations)
		lbEndpoint := envoyendpoints.LbEndpoint{
			Metadata: metadata,
			HostIdentifier: &envoyendpoints.LbEndpoint_Endpoint{
				Endpoint: &envoyendpoints.Endpoint{
					Address: &envoycore.Address{
						Address: &envoycore.Address_SocketAddress{
							SocketAddress: &envoycore.SocketAddress{
								Protocol: envoycore.SocketAddress_TCP,
								Address:  addr.Address,
								PortSpecifier: &envoycore.SocketAddress_PortValue{
									PortValue: uint32(addr.Port),
								},
							},
						},
					},
				},
			},
		}
		endpoints = append(endpoints, &lbEndpoint)
	}

	return &envoyapi.ClusterLoadAssignment{
		ClusterName: clusterName,
		Endpoints: []*envoyendpoints.LocalityLbEndpoints{{
			LbEndpoints: endpoints,
		}},
	}
}

func endpointsForUpstream(upstream *v1.Upstream, endpoints []*v1.Endpoint) []*v1.Endpoint {
	var clusterEndpoints []*v1.Endpoint
	for _, ep := range endpoints {
		for _, upstreamRef := range ep.Upstreams {
			if *upstreamRef == upstream.Metadata.Ref() {
				clusterEndpoints = append(clusterEndpoints, ep)
			}
		}
	}
	return clusterEndpoints
}

func addAnnotations(metadata *envoycore.Metadata, annotations map[string]string) *envoycore.Metadata {
	if annotations == nil {
		return metadata
	}
	if metadata == nil {
		metadata = &envoycore.Metadata{
			FilterMetadata: map[string]*structpb.Struct{},
		}
	}

	if metadata.FilterMetadata == nil {
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

	metadata.FilterMetadata[SoloAnnotations] = &structpb.Struct{
		Fields: fields,
	}
	return metadata
}

func getLbMetadata(upstream *v1.Upstream, labels map[string]string, zeroValue string) *envoycore.Metadata {

	meta := &envoycore.Metadata{
		FilterMetadata: map[string]*structpb.Struct{},
	}

	labelsStruct := &structpb.Struct{
		Fields: map[string]*structpb.Value{},
	}

	if upstream != nil {
		for _, k := range allKeys(upstream) {
			labelsStruct.Fields[k] = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: zeroValue,
				},
			}
		}
	}

	if labels != nil {
		for k, v := range labels {
			labelsStruct.Fields[k] = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: v,
				},
			}
		}
	}

	if len(labelsStruct.Fields) == 0 {
		return nil
	}

	meta.FilterMetadata[EnvoyLb] = labelsStruct
	return meta
}

func allKeys(upstream *v1.Upstream) []string {
	specGetter, ok := upstream.UpstreamType.(v1.SubsetSpecGetter)
	if !ok {
		return nil
	}
	glooSubsetConfig := specGetter.GetSubsetSpec()
	if glooSubsetConfig == nil {
		return nil
	}
	keysSet := map[string]bool{}

	for _, keys := range glooSubsetConfig.Selectors {
		for _, key := range keys.Keys {
			keysSet[key] = true
		}
	}

	var allKeys []string
	for k := range keysSet {
		allKeys = append(allKeys, k)
	}
	return allKeys
}
