<a name="top"></a>

## Contents
  - [Upstream](#v1.Upstream)
  - [ServiceInfo](#v1.ServiceInfo)
  - [Function](#v1.Function)



<a name="upstream"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="v1.Upstream"></a>

### Upstream
Upstream represents a destination for routing. Upstreams can be compared to
[clusters](https://www.envoyproxy.io/docs/envoy/latest/api-v1/cluster_manager/cluster.html?highlight=cluster) in Envoy terminology.
Upstreams can take a variety of types&lt;!--(TODO)--&gt; in gloo. Language extensions known as plugins&lt;!--(TODO)--&gt; allow the addition of new
types of upstreams. &lt;!--See [upstream types](TODO) for a detailed description of available upstream types.--&gt;


```yaml
name: string
type: string
connection_timeout: {google.protobuf.Duration}
spec: {google.protobuf.Struct}
functions: [{Function}]
status: (read only)
metadata: {Metadata}
service_info: {ServiceInfo}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | string |  | Name of the upstream. Names must be unique and follow the following syntax rules: One or more lowercase rfc1035/rfc1123 labels separated by &#39;.&#39; with a maximum length of 253 characters. |
| type | string |  | Type indicates the type of the upstream. Examples include service&lt;!--(TODO)--&gt;, kubernetes&lt;!--(TODO)--&gt;, and [aws](../plugins/aws.md) Types are defined by the plugin&lt;!--(TODO)--&gt; associated with them. |
| connection_timeout | [google.protobuf.Duration](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/duration) |  | Connection Timeout tells gloo to set a timeout for unresponsive connections created to this upstream. If not provided by the user, it will set to a default value |
| spec | [google.protobuf.Struct](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/struct) |  | Spec contains properties that are specific to the upstream type. The spec is always required, but the expected content is specified by the [upstream plugin] for the given upstream type. Most often the upstream spec will be a map&lt;string, string&gt; |
| functions | [Function](upstream.md#v1.Function) | repeated | Certain upstream types support (and may require) [functions](../introduction/concepts.md#Functions). Functions allow function-level routing to be done. For example, the [AWS lambda](../plugins/aws.md) upstream type Permits routing to AWS lambda function]. [routes](virtualservice.md#Route) on virtualservices can specify function destinations to route to specific functions. |
| status | [Status](status.md#v1.Status) |  | Status indicates the validation status of the upstream resource. Status is read-only by clients, and set by gloo during validation |
| metadata | [Metadata](metadata.md#v1.Metadata) |  | Metadata contains the resource metadata for the upstream |
| service_info | [ServiceInfo](upstream.md#v1.ServiceInfo) |  | Service Info contains information about the service running on the upstream Service Info is optional, but is used by certain plugins (such as the gRPC plugin) as well as discovery services to provide sophistocated routing features for well-known types of services |






<a name="v1.ServiceInfo"></a>

### ServiceInfo



```yaml
type: string
properties: {google.protobuf.Struct}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | string |  | Type indicates the type of service running on the upstream. Current options include `REST`, `gRPC`, and `NATS` |
| properties | [google.protobuf.Struct](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/struct) |  | Properties contains properties that describe the service. The spec may be required by the Upstream Plugin that handles the given Service Type Most often the service properties will be a map&lt;string, string&gt; |






<a name="v1.Function"></a>

### Function



```yaml
name: string
spec: {google.protobuf.Struct}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | string |  | Name of the function. Functions are referenced by name from routes and therefore must be unique within an upstream |
| spec | [google.protobuf.Struct](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/struct) |  | Spec for the function. Like [upstream specs](TODO), the content of function specs is specified by the [upstream plugin](TODO) for the upstream&#39;s type. |





 

 

 

