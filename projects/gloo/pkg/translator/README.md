# Gloo Translation

The Gloo Translator is responsible for converting a Gloo Proxy into an xDS Snapshot. It does this in the following order:

1. Compute Cluster subsystem resources (Clusters, ClusterLoadAssignments)
1. Compute Listener subsystem resources (RouteConfigurations, Listeners)
1. Generate an xDS Snapshot
1. Return the xDS Snapshot, ResourceReports and ProxyReport

## Inputs

## Cluster Subsystem Translation

### Cluster Translation

### ClusterLoadAssignment Translation

## Listener Subsystem Translation

*The [Listener subsystem](https://www.envoyproxy.io/docs/envoy/latest/intro/life_of_a_request.html?#high-level-architecture) handles downstream request processing.*

It is composed of:
1. RouteConfigurations
2. Listeners

### RouteConfiguration Translation



### Listener Translation


## Outputs