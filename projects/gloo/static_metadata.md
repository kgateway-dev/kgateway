# Uses of metadataStatic

The [Gloo Proxy Api](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk) contains a [SourceMetaData](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/#sourcemetadata) message that an an element of:
* [Listeners](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/#listener)
* [VirtualHosts](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/#listener)
* [Routes](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/#route)


This metadata is not required and the `resourceKind`, `resourceRef.name`, and `resourceRef.namespace` fields which compose the metadata are plain strings.

While the objects used to create the Proxy Api resources are and should be generally irrelevant to the functionality of Gloo Edge, they do provide user facing value as sources of names and labels.

## Current uses of this data
* Open Telementy `service.name`..
The [Open Telemetry](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/external/envoy/config/trace/v3/opentelemetry.proto.sk/#package-soloioenvoyconfigtracev3) resource has a `ServiceNameSource` field that defaults to the value `GatewayName`, which looks for a `StaticMetadataResource` with a `resourceKind` of `*v1.Gateway`, and uses the `resourceRef.name` for the `service.name` field of the Envoy `OpenTelemetryConfig`.