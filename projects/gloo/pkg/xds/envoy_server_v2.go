// Copyright 2018 Envoyproxy Authors
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

// Package server provides an implementation of a streaming xDS server.
package xds

import (
	"context"
	"errors"

	envoy_service_discovery_v2 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
)

// Server is a collection of handlers for streaming discovery requests.
type EnvoyServerV2 interface {
	v2.EndpointDiscoveryServiceServer
	v2.ClusterDiscoveryServiceServer
	v2.RouteDiscoveryServiceServer
	v2.ListenerDiscoveryServiceServer
	envoy_service_discovery_v2.AggregatedDiscoveryServiceServer
}

type envoyServerV2 struct {
	server.Server
}

// NewServer creates handlers from a config watcher and an optional logger.
func NewEnvoyServerV2(genericServer server.Server) EnvoyServerV2 {
	return &envoyServerV2{Server: genericServer}
}

func (s *envoyServerV2) StreamAggregatedResources(
	stream envoy_service_discovery_v2.AggregatedDiscoveryService_StreamAggregatedResourcesServer,
) error {
	return s.Server.StreamV2(stream, resource.AnyType)
}

func (s *envoyServerV2) StreamEndpoints(stream v2.EndpointDiscoveryService_StreamEndpointsServer) error {
	return s.Server.StreamV2(stream, resource.EndpointTypeV2)
}

func (s *envoyServerV2) StreamClusters(stream v2.ClusterDiscoveryService_StreamClustersServer) error {
	return s.Server.StreamV2(stream, resource.ClusterTypeV2)
}

func (s *envoyServerV2) StreamRoutes(stream v2.RouteDiscoveryService_StreamRoutesServer) error {
	return s.Server.StreamV2(stream, resource.RouteTypeV2)
}

func (s *envoyServerV2) StreamListeners(stream v2.ListenerDiscoveryService_StreamListenersServer) error {
	return s.Server.StreamV2(stream, resource.ListenerTypeV2)
}

func (s *envoyServerV2) FetchEndpoints(ctx context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.Unavailable, "empty request")
	}
	req.TypeUrl = resource.EndpointTypeV2
	return s.Server.FetchV2(ctx, req)
}

func (s *envoyServerV2) FetchClusters(ctx context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.Unavailable, "empty request")
	}
	req.TypeUrl = resource.ClusterTypeV2
	return s.Server.FetchV2(ctx, req)
}

func (s *envoyServerV2) FetchRoutes(ctx context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.Unavailable, "empty request")
	}
	req.TypeUrl = resource.RouteTypeV2
	return s.Server.FetchV2(ctx, req)
}

func (s *envoyServerV2) FetchListeners(ctx context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.Unavailable, "empty request")
	}
	req.TypeUrl = resource.ListenerTypeV2
	return s.Server.FetchV2(ctx, req)
}

func (s *envoyServerV2) DeltaClusters(_ v2.ClusterDiscoveryService_DeltaClustersServer) error {
	return errors.New("not implemented")
}

func (s *envoyServerV2) DeltaRoutes(_ v2.RouteDiscoveryService_DeltaRoutesServer) error {
	return errors.New("not implemented")
}

func (s *envoyServerV2) DeltaEndpoints(v2.EndpointDiscoveryService_DeltaEndpointsServer) error {
	return errors.New("not implemented")
}

func (s *envoyServerV2) DeltaListeners(v2.ListenerDiscoveryService_DeltaListenersServer) error {
	return errors.New("not implemented")
}

func (s *envoyServerV2) DeltaAggregatedResources(
	envoy_service_discovery_v2.AggregatedDiscoveryService_DeltaAggregatedResourcesServer,
) error {
	return errors.New("not implemented")
}
