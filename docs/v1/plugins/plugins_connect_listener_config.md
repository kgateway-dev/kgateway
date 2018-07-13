<a name="top"></a>

## Contents
  - [ListenerConfig](#gloo.api.connect.v1.ListenerConfig)
  - [InboundListenerConfig](#gloo.api.connect.v1.InboundListenerConfig)
  - [AuthConfig](#gloo.api.connect.v1.AuthConfig)
  - [OutboundListenerConfig](#gloo.api.connect.v1.OutboundListenerConfig)



<a name="github.com/solo-io/gloo/pkg/plugins/connect/listener_config"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.connect.v1.ListenerConfig"></a>

### ListenerConfig
the listenerConfig must be either an InboundListener or an OutboundListener


```yaml
inbound: {InboundListenerConfig}
outbound: {OutboundListenerConfig}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| inbound | [InboundListenerConfig](github.com/solo-io/gloo/pkg/plugins/connect/listener_config.md#gloo.api.connect.v1.InboundListenerConfig) |  |  |
| outbound | [OutboundListenerConfig](github.com/solo-io/gloo/pkg/plugins/connect/listener_config.md#gloo.api.connect.v1.OutboundListenerConfig) |  |  |






<a name="gloo.api.connect.v1.InboundListenerConfig"></a>

### InboundListenerConfig
configuration for the inbound listener
this listener does authentication and connects
clients to the local service


```yaml
auth_config: {AuthConfig}
local_service_address: string
local_service_name: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| auth_config | [AuthConfig](github.com/solo-io/gloo/pkg/plugins/connect/listener_config.md#gloo.api.connect.v1.AuthConfig) |  | configuration for tls-based auth filter |
| local_service_address | string |  | the address of the local upstream being proxied the service being proxied must be reachable by Envoy |
| local_service_name | string |  | the name of the local consul service being proxied |






<a name="gloo.api.connect.v1.AuthConfig"></a>

### AuthConfig
AuthConfig contains information necessary to
communicate with the Authentication Server (Consul Agent)


```yaml
target: string
authorize_hostname: string
authorize_port: uint32
authorize_path: string
request_timeout: {google.protobuf.Duration}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target | string |  | The name of the service who owns this proxy Target must be delivered by the filter as part of the authorize request payload |
| authorize_hostname | string |  | the hostname of the authorization REST service |
| authorize_port | uint32 |  | the port of the authorization REST service |
| authorize_path | string |  | the request path for the authorization REST service NOTE: currently ignored by the plugin and filter |
| request_timeout | [google.protobuf.Duration](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/duration) |  | Connection Timeout tells the filter to set a timeout for unresponsive connections created to this upstream. If not provided by the user, it will set to a default value |






<a name="gloo.api.connect.v1.OutboundListenerConfig"></a>

### OutboundListenerConfig
The configuration for the outbound listeners which serve as &#34;tcp routes&#34;


```yaml
destination_consul_service: string
destination_consul_type: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| destination_consul_service | string |  | The name of the consul service which is the destination for the listener |
| destination_consul_type | string |  | TODO (ilackarms): support destination type in Consul Connect API |





 

 

 

