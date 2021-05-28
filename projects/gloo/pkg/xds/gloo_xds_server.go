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
	"errors"

	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	gloo_discovery_service "github.com/solo-io/solo-kit/pkg/api/xds"

	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
)

// Server is a collection of handlers for streaming discovery requests.
type GlooXdsServer interface {
	gloo_discovery_service.GlooDiscoveryServiceServer
}

type glooXdsServer struct {
	server.Server
}

// NewServer creates handlers from a config watcher and an optional logger.
func NewGlooXdsServer(genericServer server.Server) GlooXdsServer {
	return &glooXdsServer{Server: genericServer}
}

func (s *glooXdsServer) StreamAggregatedResources(
	stream gloo_discovery_service.GlooDiscoveryService_StreamAggregatedResourcesServer,
) error {
	return s.Server.StreamV2(stream, resource.AnyType)
}

func (s *glooXdsServer) DeltaAggregatedResources(
	gloo_discovery_service.GlooDiscoveryService_DeltaAggregatedResourcesServer,
) error {
	return errors.New("not implemented")
}
