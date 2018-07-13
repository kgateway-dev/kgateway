<a name="top"></a>

## Contents
  - [RouteExtensions](#gloo.api.v1.RouteExtensions)
  - [HeaderValue](#gloo.api.v1.HeaderValue)
  - [CorsPolicy](#gloo.api.v1.CorsPolicy)



<a name="github.com/solo-io/gloo/pkg/coreplugins/routing/route_extensions"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.v1.RouteExtensions"></a>

### RouteExtensions
RouteExtensions should be placed in the route.extensions field
RouteExtensions extend the behavior of a regular route in gloo (within a virtual service)


```yaml
add_request_headers: [{HeaderValue}]
add_response_headers: [{HeaderValue}]
remove_response_headers: [string]
max_retries: uint32
timeout: {google.protobuf.Duration}
host_rewrite: string
cors: {CorsPolicy}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| add_request_headers | [HeaderValue](github.com/solo-io/gloo/pkg/coreplugins/routing/route_extensions.md#gloo.api.v1.HeaderValue) | repeated | These headers will be added to the request before it is sent to the upstream |
| add_response_headers | [HeaderValue](github.com/solo-io/gloo/pkg/coreplugins/routing/route_extensions.md#gloo.api.v1.HeaderValue) | repeated | These headers will be added to the response before it is returned to the downstream |
| remove_response_headers | string | repeated | These headers will be removed from the request before it is sent to the upstream |
| max_retries | uint32 |  | The maximum number of retries to attempt for requests that get a 5xx response |
| timeout | [google.protobuf.Duration](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/duration) |  | If set, time out requests on this route. If not set, this will default to the connection timeout on the upstream |
| host_rewrite | string |  | Rewrite the host header of the request to this value, if set |
| cors | [CorsPolicy](github.com/solo-io/gloo/pkg/coreplugins/routing/route_extensions.md#gloo.api.v1.CorsPolicy) |  | Configure Cross-Origin Resource Sharing requests |






<a name="gloo.api.v1.HeaderValue"></a>

### HeaderValue
Header name/value pair


```yaml
key: string
value: string
append: bool

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | string |  | Header name |
| value | string |  | Header value |
| append | bool |  | Should this value be appended? |






<a name="gloo.api.v1.CorsPolicy"></a>

### CorsPolicy
Configuration for Cross-Origin Resource Sharing requests


```yaml
allow_origin: [string]
allow_methods: string
allow_headers: string
expose_headers: string
max_age: {google.protobuf.Duration}
allow_credentials: bool

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| allow_origin | string | repeated | Specifies the origins that will be allowed to do CORS requests. An origin is allowed if either allow_origin matches. |
| allow_methods | string |  | Specifies the content for the *access-control-allow-methods* header. |
| allow_headers | string |  | Specifies the content for the *access-control-allow-headers* header. |
| expose_headers | string |  | Specifies the content for the *access-control-expose-headers* header. |
| max_age | [google.protobuf.Duration](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/duration) |  | Specifies the content for the *access-control-max-age* header. |
| allow_credentials | bool |  | Specifies whether the resource allows credentials. |





 

 

 

