# Endpoint Discovery Service (EDS)

## Overview
Envoy supports a variety of mechanisms to configure [Service Discovery](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/service_discovery#arch-overview-service-discovery), the mechanism Envoy uses to resolve the members of the cluster. One of the more complex mechanisms is [EDS](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/service_discovery#endpoint-discovery-service-eds), where the xDS management server (Gloo Edge) provides the Endpoints for a given Cluster via an API.

In Gloo Edge, Endpoints can be discovered dynamically via the Endpoint Discovery feature, and then served to the Envoy proxy via the EDS API.

## How it works
EDS runs in the [Gloo](/projects/gloo) component of Gloo Edge.

We rely on an [In Memory Endpoint Client](https://github.com/solo-io/gloo/blob/a39fb91c2fb122d5a34353dff891e0b0044bf1dc/projects/gloo/pkg/syncer/setup/setup_syncer.go#L469), meaning that when we “write” an Endpoint, it is just persisted in memory. In Memory clients signal updates to the emitter by two mechanisms: 
- [timed update](https://github.com/solo-io/solo-kit/blob/1f34a76bf919fd40a50e4504a837ee4b41b7f215/pkg/api/v1/clients/memory/resource_client.go#L238)
- [signaled update](https://github.com/solo-io/solo-kit/blob/1f34a76bf919fd40a50e4504a837ee4b41b7f215/pkg/api/v1/clients/memory/resource_client.go#L240)

_Any writes are signaled, meaning that endpoint updates will trigger the resources being sent on the channel, which will be picked up by the emitter, and produce a new snapshot._

Create an [EDS Event Loop](https://github.com/solo-io/gloo/blob/a39fb91c2fb122d5a34353dff891e0b0044bf1dc/projects/gloo/pkg/syncer/setup/setup_syncer.go#L626), which is responsible for the following:
- Process an [EDS Snapshot](https://github.com/solo-io/gloo/blob/a39fb91c2fb122d5a34353dff891e0b0044bf1dc/projects/gloo/pkg/api/v1/eds_snapshot.sk.go#L18)
- Performs a [Sync](https://github.com/solo-io/gloo/blob/a39fb91c2fb122d5a34353dff891e0b0044bf1dc/projects/gloo/pkg/discovery/run.go#L31) on each iteration
- Each Sync will perform a new [EDS Sync](https://github.com/solo-io/gloo/blob/a39fb91c2fb122d5a34353dff891e0b0044bf1dc/projects/gloo/pkg/discovery/discovery.go#L173) and any existing loops will be cancelled

An EDS Sync is responsible for the following:
- Initiate a watch on the Upstreams, which produces a [channel of Endpoints](https://github.com/solo-io/gloo/blob/a39fb91c2fb122d5a34353dff891e0b0044bf1dc/projects/gloo/pkg/discovery/discovery.go#L177)
- Open a goroutine to [reconcile the Endpoints](https://github.com/solo-io/gloo/blob/a39fb91c2fb122d5a34353dff891e0b0044bf1dc/projects/gloo/pkg/discovery/discovery.go#L197)

## EDS Plugins
Similar to [UDS](./upstream-discovery.md), EDS supports multiple plugins for discovering endpoints. 

Each plugin is responsible for producing Gloo Endpoints, and sending them back to the EDS loop to be persisted in our in-memory API Snapshot. Then during translation, the Gloo Endpoints and Gloo Upstreams are converted into Envoy Endpoints and Envoy Clusters.

## Configuration

### Settings Resource
EDS behavior can be configured globally, in the [Settings](/projects/gloo/api/v1/settings.proto) resource under `refresh_rate`.

Each EDS plugin may have additional configuration options on the Settings resource as well.

