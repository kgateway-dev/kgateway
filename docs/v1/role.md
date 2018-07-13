<a name="top"></a>

## Contents
  - [Role](#gloo.api.v1.Role)
  - [Listener](#gloo.api.v1.Listener)



<a name="role"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.v1.Role"></a>

### Role
A Role is a container for a set of Virtual Services that will be used to generate a single proxy config
to be applied to one or more Envoy nodes. The Role is best understood as an in-mesh application&#39;s localized view
of the rest of the mesh.
Each domain for each Virtual Service contained in a Role cannot appear more than once, or the Role
will be invalid.
Roles contain a config field which can be written to for the purpose of applying configuration and policy
to groupings of Virtual Services.


```yaml
name: string
listeners: [{Listener}]
status: (read only)
metadata: {Metadata}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | string |  | Name of the role. Envoy nodes will be assigned a config matching the role they report to Gloo when registering Envoy instances must specify their role in the prefix for their Node ID when they register to Gloo. Currently this is done in the format &lt;Role&gt;~&lt;this portion is ignored&gt; which can be specified with the `--service-node` flag, or in the Envoy instance&#39;s bootstrap config. Role Names must be unique and follow the following syntax rules: One or more lowercase rfc1035/rfc1123 labels separated by &#39;.&#39; with a maximum length of 253 characters. |
| listeners | [Listener](role.md#gloo.api.v1.Listener) | repeated | define each listener the proxy will create listeners define a set of behaviors for a single address:port where the proxy will listen if no listeners are specified, the role will behave as a gateway see (pkg/api/defaults/v1)[https://github.com/solo-io/gloo/tree/master/pkg/api/defaults/v1] to see the default listeners that will be created for Gateway proxies binding to the default HTTP (8080) and HTTPS (8443) ports on 0.0.0.0 (all interfaces) |
| status | [Status](status.md#gloo.api.v1.Status) |  | Status indicates the validation status of the role resource. Status is read-only by clients, and set by gloo during validation |
| metadata | [Metadata](metadata.md#gloo.api.v1.Metadata) |  | Metadata contains the resource metadata for the role |






<a name="gloo.api.v1.Listener"></a>

### Listener
Listeners define the address:port where the proxy will listen for incoming connections
Each listener defines a unique set of TCP and HTTP behaviors


```yaml
name: string
bind_address: string
bind_port: uint32
virtual_services: [string]
config: {google.protobuf.Struct}
labels: map<string,string>
ssl_config: {SSLConfig}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | string |  | the name of the listener. names must be unique for listeners within a role |
| bind_address | string |  | the bind address for the listener. both ipv4 and ipv6 formats are supported |
| bind_port | uint32 |  | the port to bind on ports numbers must be unique for listeners within a role |
| virtual_services | string | repeated | defines the set of virtual services that will be accessible by clients connecting to this listener. at least one virtual service must be specifiedfor HTTP-level features to be applied at the listener level |
| config | [google.protobuf.Struct](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/struct) |  | Config contains top-level config to be applied to a listener Listener config is applied to all TCP/HTTP traffic that initiates via this listener. Configuration such as gzip compression and TLS authentication is specified here |
| labels | map&lt;string,string&gt; |  | Apply Listener Attributes to listeners with selectors matching these label keys and values If empty or not present, the Listener will inherit no configuration from Attributes. |
| ssl_config | [SSLConfig](virtualservice.md#gloo.api.v1.SSLConfig) |  | SSL Config is optional for the role. If provided, the listener will serve TLS for connections on this port this is useful when there are no virtual services assigned to this listener, e.g. for the purpose of securing a Listener functioning as a TCP Proxy if no virtual services are defined and ssl_config is nil, the proxy will serve tcp connections insecurely on this port |





 

 

 

