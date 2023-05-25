# Translator

The Gloo [Translator](./translator.go) is responsible for converting a Gloo Proxy into an xDS Snapshot. It does this in the following order:

1. Compute Cluster subsystem resources (Clusters, ClusterLoadAssignments)
1. Compute Listener subsystem resources (RouteConfigurations, Listeners)
1. Generate an xDS Snapshot
1. Return the xDS Snapshot, ResourceReports and ProxyReport

## Inputs

### ApiSnapshot

The [ApiSnapshot](https://github.com/solo-io/gloo/blob/bc380b36d42fdad7c83ab8dc4f055258b326aeac/projects/gloo/pkg/api/v1/gloosnapshot/api_snapshot.sk.go#L22) represents the state of the world, according to the Gloo controller.

It is a generated file, constructed from a [template](https://github.com/solo-io/solo-kit/blob/97bd7c2c67420a6d99bb96f220f2e1a04c6d8a0d/pkg/code-generator/codegen/templates/snapshot_template.go) that aggregates all resources for a given project.

### Proxy

The [Proxy](https://github.com/solo-io/gloo/blob/bc380b36d42fdad7c83ab8dc4f055258b326aeac/projects/gloo/api/v1/proxy.proto#L42) is a container for the entire set of configuration that will to be applied to one or more [Envoy Proxy instances](https://github.com/solo-io/gloo/tree/main/projects/envoyinit).

## Cluster Subsystem Translation

*The [Cluster subsystem](https://www.envoyproxy.io/docs/envoy/latest/intro/life_of_a_request.html?#high-level-architecture) is responsible for selecting and configuring the upstream connection to an endpoint.*

It is composed of:
1. Clusters
1. ClusterLoadAssignments (Endpoints)

## Listener Subsystem Translation

*The [Listener subsystem](https://www.envoyproxy.io/docs/envoy/latest/intro/life_of_a_request.html?#high-level-architecture) handles downstream request processing.*

It is composed of:
1. RouteConfigurations
1. Listeners

## Outputs

### xDS Snapshot

Context around the xDS Snapshot and other xDS concepts can be found in the [xDS package](../xds)

### ResourceReports

[ResourceReports](https://github.com/solo-io/solo-kit/blob/97bd7c2c67420a6d99bb96f220f2e1a04c6d8a0d/pkg/api/v2/reporter/reporter.go#L24) are an aggregated set of errors and warnings that are accumulated during translation. These allow translation to complete before flagging resources as having [an errored or warning state](https://docs.solo.io/gloo-edge/latest/guides/traffic_management/configuration_validation/#warnings-and-errors)

### ProxyReport

[ProxyReport](https://github.com/solo-io/gloo/blob/1f457f4ef5f32aedabc58ef164aeea92acbf481e/projects/gloo/pkg/api/grpc/validation/gloo_validation.pb.go#L837) is an aggregated set of reports for all sub-resources of a Proxy.