# Upstream Discovery Service (UDS)

## Overview
A [Gloo Upstream](/projects/gloo/api/v1/upstream.proto) represents a destination for routing HTTP requests. An Upstream can be compared to [clusters](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto) in Envoy terminology.

Upstreams can either be defined statically in a cluster, or can be discovered dynamically via Gloo Edge's Upstream Discovery feature.

## How it works
UDS runs in the [Discovery](/projects/discovery) component of Gloo Edge.

Create a [discovery emitter](https://github.com/solo-io/gloo/blob/8bbe175ea136178bfe8b4d103ae702d4965c4c75/projects/gloo/pkg/api/v1/discovery_snapshot_emitter.sk.go#L135), which is responsible for the following:
- Emit a snapshot of the resources that are required for UDS to operate (Upstreams, Secrets, Kubernetes Namespaces)
- This set of resources is referred to as the [DiscoverySnapshot](https://github.com/solo-io/gloo/blob/8bbe175ea136178bfe8b4d103ae702d4965c4c75/projects/gloo/pkg/api/v1/discovery_snapshot.sk.go#L20)

Create and start a [UDS Engine](https://github.com/solo-io/gloo/blob/8bbe175ea136178bfe8b4d103ae702d4965c4c75/projects/discovery/pkg/uds/syncer/setup_syncer.go#L91), which is responsible for the following:
- Open a goroutine for each type of Discovery plugin (Kubernetes, ec2..etc)
- Each plugin then discovers the corresponding Upstreams, and stores them in an [in memory map on the UDS Engine](https://github.com/solo-io/gloo/blob/8bbe175ea136178bfe8b4d103ae702d4965c4c75/projects/gloo/pkg/discovery/discovery.go#L115)
- Each time that the in-memory map is updated, the UDS Engine Resyncs, meaning it attempts to reconcile all the upstreams

Create and start the [UDS Event Loop](https://github.com/solo-io/gloo/blob/8bbe175ea136178bfe8b4d103ae702d4965c4c75/projects/discovery/pkg/uds/syncer/setup_syncer.go#L99):
- This follows our standard pattern of an Event Loop / Emitter / Syncer, where the event loop watches events, uses the emitter to producer a snapshot, and sends that Snapshot to the Sync to be Synced
- In this case, a “Sync”, just invokes [Resync](https://github.com/solo-io/gloo/blob/8bbe175ea136178bfe8b4d103ae702d4965c4c75/projects/gloo/pkg/discovery/discovery.go#L132) on the UDS Engine. That method takes the up to date in memory upstreams, and reconciles them

## UDS Plugins
UDS supports multiple plugins for discovering upstreams. To determine which types of services are supported, search for all [DiscoveryPlugins](https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/discovery/discovery.go#L28).

Each plugin is responsible for discovering upstreams of a specific type. For example, the [Kubernetes Discovery Plugin](/projects/gloo/pkg/plugins/kubernetes/uds.go) is responsible for discovering Kubernetes Services, and generating the corresponding Upstreams.

## Configuration

### Settings Resource
UDS behavior can be configured globally, in the [Settings](/projects/gloo/api/v1/settings.proto) resource under `discovery.uds_options`.

