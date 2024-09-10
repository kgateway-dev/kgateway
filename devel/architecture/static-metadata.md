# Uses of metadataStatic

The [Gloo Proxy Api](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk) contains a [SourceMetaData](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/#sourcemetadata) message that an an element of:
* [Listeners](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/#listener)
* [VirtualHosts](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/#listener)
* [Routes](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/#route)


This metadata is not required and the `resourceKind`, `resourceRef.name`, and `resourceRef.namespace` fields which compose the metadata are plain strings.

While the objects used to create the Proxy Api resources are and should be generally irrelevant to the functionality of Gloo Gateway, they do provide user facing value as sources of names and labels.

## Current uses of this data
### Open Telemetry `service.name`
 When creating a [Envoy OpenTelemetryConfig](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/trace/v3/opentelemetry.proto.html) resource, we use the 
 static metadata to determine the value to use for the `service_name` field. A `StaticMetadataResource` is considered to be a Gateway when it has a `resourceKind` of `*v1.Gateway`. The value of that resource's `resourceRef.name` is then used as the  `service.name`.

If the metadata is not present, or in a different format, the `service_name` will be set to the following:
- Metadata is in deprecated format: "deprecated_metadata"
- Unknown metadata format: "unknown_metadata"
- Gateway metadata is not found: "undefined_gateway"