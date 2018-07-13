<a name="top"></a>

## Contents
  - [UpstreamSpec](#gloo.api.v1.UpstreamSpec)
  - [Host](#gloo.api.v1.Host)



<a name="github.com/solo-io/gloo/pkg/coreplugins/static/upstream_spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.v1.UpstreamSpec"></a>

### UpstreamSpec
Configuration for Static Upstreams


```yaml
hosts: [{Host}]
enable_ipv6: bool
tls: {google.protobuf.BoolValue}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hosts | [Host](github.com/solo-io/gloo/pkg/coreplugins/static/upstream_spec.md#gloo.api.v1.Host) | repeated | A list of addresses and ports at least one must be specified |
| enable_ipv6 | bool |  | Enable ipv6 addresses to be used for routing |
| tls | [google.protobuf.BoolValue](github.com/solo-io/gloo/pkg/coreplugins/static/upstream_spec.md#google.protobuf.BoolValue) |  | Attempt to use outbound TLS |






<a name="gloo.api.v1.Host"></a>

### Host
Represents a single instance of an upstream


```yaml
addr: string
port: uint32

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| addr | string |  | Address (hostname or IP) |
| port | uint32 |  | Port the instance is listening on |





 

 

 

