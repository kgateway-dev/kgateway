package server

import (
	"context"
	"errors"

	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
)

// Server is a collection of handlers for streaming discovery requests.
type EnvoyServerV3 interface {
	envoy_service_endpoint_v3.EndpointDiscoveryServiceServer
	envoy_service_cluster_v3.ClusterDiscoveryServiceServer
	envoy_service_route_v3.RouteDiscoveryServiceServer
	envoy_service_listener_v3.ListenerDiscoveryServiceServer
	envoy_service_discovery_v3.AggregatedDiscoveryServiceServer
}

type envoyServerV3 struct {
	server.Server
}

func NewEnvoyServerV3(genericServer server.Server) EnvoyServerV3 {
	return &envoyServerV3{Server: genericServer}
}

func (s *envoyServerV3) StreamAggregatedResources(
	stream envoy_service_discovery_v3.AggregatedDiscoveryService_StreamAggregatedResourcesServer,
) error {
	return s.Server.StreamEnvoyV3(stream, types.AnyType)
}

func (s *envoyServerV3) StreamEndpoints(
	stream envoy_service_endpoint_v3.EndpointDiscoveryService_StreamEndpointsServer,
) error {
	return s.Server.StreamEnvoyV3(stream, types.EndpointTypeV3)
}

func (s *envoyServerV3) StreamClusters(
	stream envoy_service_cluster_v3.ClusterDiscoveryService_StreamClustersServer,
) error {
	return s.Server.StreamEnvoyV3(stream, types.ClusterTypeV3)
}

func (s *envoyServerV3) StreamRoutes(
	stream envoy_service_route_v3.RouteDiscoveryService_StreamRoutesServer,
) error {
	return s.Server.StreamEnvoyV3(stream, types.RouteTypeV3)
}

func (s *envoyServerV3) StreamListeners(
	stream envoy_service_listener_v3.ListenerDiscoveryService_StreamListenersServer,
) error {
	return s.Server.StreamEnvoyV3(stream, types.ListenerTypeV3)
}

func (s *envoyServerV3) FetchEndpoints(
	ctx context.Context,
	req *envoy_service_discovery_v3.DiscoveryRequest,
) (*envoy_service_discovery_v3.DiscoveryResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.Unavailable, "empty request")
	}
	req.TypeUrl = types.EndpointTypeV3
	return s.Server.FetchEnvoyV3(ctx, req)
}

func (s *envoyServerV3) FetchClusters(
	ctx context.Context,
	req *envoy_service_discovery_v3.DiscoveryRequest,
) (*envoy_service_discovery_v3.DiscoveryResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.Unavailable, "empty request")
	}
	req.TypeUrl = types.ClusterTypeV3
	return s.Server.FetchEnvoyV3(ctx, req)
}

func (s *envoyServerV3) FetchRoutes(
	ctx context.Context,
	req *envoy_service_discovery_v3.DiscoveryRequest,
) (*envoy_service_discovery_v3.DiscoveryResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.Unavailable, "empty request")
	}
	req.TypeUrl = types.RouteTypeV3
	return s.Server.FetchEnvoyV3(ctx, req)
}

func (s *envoyServerV3) FetchListeners(
	ctx context.Context,
	req *envoy_service_discovery_v3.DiscoveryRequest,
) (*envoy_service_discovery_v3.DiscoveryResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.Unavailable, "empty request")
	}
	req.TypeUrl = types.ListenerTypeV3
	return s.Server.FetchEnvoyV3(ctx, req)
}

func (s *envoyServerV3) DeltaEndpoints(envoy_service_endpoint_v3.EndpointDiscoveryService_DeltaEndpointsServer) error {
	return errors.New("not implemented")
}

func (s *envoyServerV3) DeltaClusters(envoy_service_cluster_v3.ClusterDiscoveryService_DeltaClustersServer) error {
	return errors.New("not implemented")
}

func (s *envoyServerV3) DeltaRoutes(envoy_service_route_v3.RouteDiscoveryService_DeltaRoutesServer) error {
	return errors.New("not implemented")
}

func (s *envoyServerV3) DeltaListeners(envoy_service_listener_v3.ListenerDiscoveryService_DeltaListenersServer) error {
	return errors.New("not implemented")
}

func (s *envoyServerV3) DeltaAggregatedResources(
	envoy_service_discovery_v3.AggregatedDiscoveryService_DeltaAggregatedResourcesServer,
) error {
	return errors.New("not implemented")
}
