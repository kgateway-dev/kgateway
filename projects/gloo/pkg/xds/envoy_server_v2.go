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

	envoy_service_discovery_v2 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"

	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
)

// Server is a collection of handlers for streaming discovery requests.
type EnvoyServerV2 interface {
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

func (s *envoyServerV2) DeltaAggregatedResources(
	envoy_service_discovery_v2.AggregatedDiscoveryService_DeltaAggregatedResourcesServer,
) error {
	return errors.New("not implemented")
}
