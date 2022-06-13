# xDS

## Background

[xDS](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol) is the set of discovery services and APIs used by Envoy to discover its dynamic resources.

## xDS Server

Gloo Edge is an xDS server. It maintains a snapshot-based, in-memory cache and responds to xDS requests with the resources that are requested.

### Snapshot

A [snapshot](https://github.com/solo-io/solo-kit/blob/97bd7c2c67420a6d99bb96f220f2e1a04c6d8a0d/pkg/api/v1/control-plane/cache/snapshot.go#L43) is a versioned group of resources. In Gloo Edge, we rely on an [Envoy snapshot](https://github.com/solo-io/gloo/blob/1f457f4ef5f32aedabc58ef164aeea92acbf481e/projects/gloo/pkg/xds/envoy_snapshot.go#L39), a snapshot of the xDS resources that Gloo serves to Envoy.

### Snapshot Cache

A [snapshot cache](https://github.com/solo-io/solo-kit/blob/97bd7c2c67420a6d99bb96f220f2e1a04c6d8a0d/pkg/api/v1/control-plane/cache/simple.go#L70) maintains a single versioned snapshot per key. It also responds to open xDS requests.

### xDS Callbacks

[xDS callbacks](https://github.com/solo-io/solo-kit/blob/97bd7c2c67420a6d99bb96f220f2e1a04c6d8a0d/pkg/api/v1/control-plane/server/generic_server.go#L76) are a set of callbacks that are invoked asynchronously during the lifecycle of an xDS request.

Gloo Edge open source does not define any xDS callbacks. However, these callbacks are a type of [extension that can be injected at runtime](https://github.com/solo-io/gloo/blob/75c0ee0f3b70258d0013364e82489f570685e1d7/projects/gloo/pkg/syncer/setup/setup_syncer.go#L393). Gloo Edge Enterprise defines xDS callbacks, and injects them into the Control Plane at runtime.

### Server

An [xDS server](https://github.com/solo-io/solo-kit/blob/97bd7c2c67420a6d99bb96f220f2e1a04c6d8a0d/pkg/api/v1/control-plane/server/generic_server.go#L52) defines a set of handlers for streaming discovery requests.

## xDS Services

The xDS server is configured to expose the following discovery services in Gloo Edge:

### ListenerDiscoveryService

The [ListenerDiscoveryService](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration#lds) allows Envoy to discover Listeners at runtime.

### RouteDiscoveryService

The [RouteDiscoveryService](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration#rds) allows Envoy to discover routing configuration for an [HttpConnectionManager](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/http/http_connection_management.html) filter at runtime.

### ClusterDiscoveryService

The [ClusterDiscoveryService](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration#cds) allows Envoy to discover routable destinations at runtime.

### EndpointDiscoveryService

The [EndpointDiscoveryService](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration#eds) allows Envoy to discover members in a cluster at runtime.

### AggregatedDiscoveryService

The [AggregatedDiscoveryService](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration#aggregated-xds-ads) allows Envoy to discover all resource types over a single stream at runtime.

### SoloDiscoveryService

The [SoloDiscoveryService](https://github.com/solo-io/solo-kit/blob/97bd7c2c67420a6d99bb96f220f2e1a04c6d8a0d/api/xds/solo-discovery-service.proto#L21) is a custom xDS service, used to serve resources of Any type, that is based on Envoy's Aggregated Discovery Service.

In addition to serving configuration for Envoy resources, the Gloo xDS server is also responsible for serving configuration to a number of enterprise extensions (ie `ext-auth` and `rate-limit`)

The SoloDiscoveryService is required to serve these extension resources. It is largely based on the Envoy v2 API, and since it is purely an internal API, we do not need to upgrade the API to match the Envoy xDS API. [This issue](https://github.com/solo-io/gloo/issues/4369) contains additional context around the reason behind this custom discovery service.

## xDS Requests

Gloo Edge supports managing configuration for multiple proxies through a single xDS server. To do so, it stores each snapshot in the cache at a key that is unique to that proxy.

To guarantee that proxies initiate requests for the snapshot they want, it is critical that we establish a naming pattern for cache keys. This pattern must be used both by the proxies requesting the resources from the cache, and the controllers that set the resources in the cache.

**The naming convention that we follow is "NAMESPACE~NAME"**

Proxies identify the cache key that they are interested in by specifying their `node.metadata.role` to the cache key using the above naming pattern. An example of this can be found in the [bootstrap configuration for proxies](https://github.com/solo-io/gloo/blob/0eec04dc0486976fc89bac314b0fd9eccd5261f5/install/helm/gloo/templates/9-gateway-proxy-configmap.yaml#L45)

## xDS Debugging

Debugging xDS behavior can be challenging, below are a few techniques to help:

### Control Plane Logging

The Gloo translation loop is responsible for converting a [Gloo API Snapshot](https://github.com/solo-io/gloo/blob/6994b4108c1d8d8c33404ece16ef1249e0af920c/projects/gloo/pkg/api/v1/gloosnapshot/api_snapshot.sk.go#L22) into an [xDS Snapshot](https://github.com/solo-io/solo-kit/blob/97bd7c2c67420a6d99bb96f220f2e1a04c6d8a0d/pkg/api/v1/control-plane/cache/snapshot.go#L43). After completing a translation loop, there is a [log line](https://github.com/solo-io/gloo/blob/6994b4108c1d8d8c33404ece16ef1249e0af920c/projects/gloo/pkg/syncer/envoy_translator_syncer.go#L166) indicating what key in the snapshot cache was updated.

### xDS Debug Endpoint

Gloo supports running in [dev mode](https://github.com/solo-io/gloo/blob/6994b4108c1d8d8c33404ece16ef1249e0af920c/projects/gloo/pkg/syncer/setup/setup_syncer.go#L360), and when that is enabled, [xDS Snapshots](https://github.com/solo-io/gloo/blob/6994b4108c1d8d8c33404ece16ef1249e0af920c/projects/gloo/pkg/syncer/translator_syncer.go#L96) are exposed via an endpoint.

## Useful information

- [Hoot YouTube series about xDS](https://www.youtube.com/watch?v=S5Fm1Yhomc4)
- [Envoy xDS](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol)
- [Hoot Repository](https://github.com/solo-io/hoot)