package translator

import (
	"context"

	"go.opencensus.io/trace"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyendpoints "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// Endpoints

func computeClusterEndpoints(ctx context.Context, upstreams []*v1.Upstream, endpoints []*v1.Endpoint) []*envoyapi.ClusterLoadAssignment {

	_, span := trace.StartSpan(ctx, "gloo.translator.computeClusterEndpoints")
	defer span.End()

	var clusterEndpointAssignments []*envoyapi.ClusterLoadAssignment
	for _, upstream := range upstreams {
		clusterEndpoints := endpointsForUpstream(upstream, endpoints)
		// if there are any endpoints for this upstream, it's using eds and we need to create a load assignment for it
		if len(clusterEndpoints) > 0 {
			loadAssignment := loadAssignmentForCluster(UpstreamToClusterName(upstream.Metadata.Ref()), clusterEndpoints)
			clusterEndpointAssignments = append(clusterEndpointAssignments, loadAssignment)
		}
	}
	return clusterEndpointAssignments
}

func loadAssignmentForCluster(clusterName string, clusterEndpoints []*v1.Endpoint) *envoyapi.ClusterLoadAssignment {
	var endpoints []envoyendpoints.LbEndpoint
	for _, addr := range clusterEndpoints {
		lbEndpoint := envoyendpoints.LbEndpoint{
			Endpoint: &envoyendpoints.Endpoint{
				Address: &envoycore.Address{
					Address: &envoycore.Address_SocketAddress{
						SocketAddress: &envoycore.SocketAddress{
							Protocol: envoycore.TCP,
							Address:  addr.Address,
							PortSpecifier: &envoycore.SocketAddress_PortValue{
								PortValue: uint32(addr.Port),
							},
						},
					},
				},
			},
		}
		endpoints = append(endpoints, lbEndpoint)
	}

	return &envoyapi.ClusterLoadAssignment{
		ClusterName: clusterName,
		Endpoints: []envoyendpoints.LocalityLbEndpoints{{
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
