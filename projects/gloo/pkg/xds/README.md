# xDS

## Background

[xDS](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol) is the set of discovery services and APIs used by Envoy to discover its dynamic resources.

## xDS Server

Gloo Edge is an xDS server. It maintains a snapshot-based, in-memory cache and responds to xDS requests with the resources that are requested.

The following discovery services are supported by Gloo Edge:

### ListenerDiscoveryService

The [ListenerDiscoveryService](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration#lds) allows Envoy to discovery Listeners at runtime.

### RouteDiscoveryService

The [RouteDiscoveryService](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration#rds) allows Envoy to discovery routing configuration for an [HttpConnectionManager](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/http/http_connection_management.html) filter at runtime.

### ClusterDiscoveryService

The [ClusterDiscoveryService](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration#cds) allows Envoy to discovery routable destinations at runtime.

### EndpointDiscoveryService

The [EndpointDiscoveryService](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration#eds) allows Envoy to discovery members in a cluster at runtime.

### AggregatedDiscoveryService

The [AggregatedDiscoveryService](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration#aggregated-xds-ads) allows Envoy to discovery all resource types over a single stream at runtime.

### SoloDiscoveryService

The [SoloDiscoveryService](https://github.com/solo-io/solo-kit/blob/97bd7c2c67420a6d99bb96f220f2e1a04c6d8a0d/pkg/api/xds/solo-discovery-service.pb.go#L194) is a custom xDS service, used to serve resources of Any type.

In addition to serving configuration for Envoy resources, the Gloo xDS server is also responsible for serving configuration to a number of enterprise extensions (ie `ext-auth` and `rate-limit`)

The SoloDiscoveryService is required to serve these extension resources. It is largely based on the Envoy v2 API, and since it is purely an internal API, we do not need to upgrade the API as the Envoy xDS API. [This issue](https://github.com/solo-io/gloo/issues/4369) contains additional context around the reason behind this custom discovery service.

## xDS Requests

Gloo Edge supports managing configuration for multiple proxies through a single xDS server. To do so, it stores each snapshot in the cache at a key that is unique to that proxy.

To guarantee that proxies initiate requests for the snapshot they want, it is critical that we establish a naming pattern for cache keys. This pattern must be used both by the proxies requesting the resources from the cache, and the controllers that set the resources in the cache.

**The naming convention that we follow is "NAMESPACE~NAME"**

Proxies identify the cache key that they are interested in by specifying their `node.metadata.role` to the cache key using the above naming pattern. An example of this can be found in the [bootstrap configuration for proxies](https://github.com/solo-io/gloo/blob/0eec04dc0486976fc89bac314b0fd9eccd5261f5/install/helm/gloo/templates/9-gateway-proxy-configmap.yaml#L45)

## xDS Callbacks

[xDS callbacks](https://github.com/solo-io/solo-kit/blob/97bd7c2c67420a6d99bb96f220f2e1a04c6d8a0d/pkg/api/v1/control-plane/server/generic_server.go#L76) are a set of callbacks that are invoked asynchronously during the lifecycle of an xDS request.

Gloo Edge open source does not define any xDS callbacks. However, these callbacks are a type of [extension that can be injected at runtime](https://github.com/solo-io/gloo/blob/75c0ee0f3b70258d0013364e82489f570685e1d7/projects/gloo/pkg/syncer/setup/setup_syncer.go#L393). Gloo Edge Enterprise defines xDS callbacks, and injects them into the Control Plane at runtime.

## Useful information

- [Hoot YouTube series about xDS](https://www.youtube.com/watch?v=S5Fm1Yhomc4)