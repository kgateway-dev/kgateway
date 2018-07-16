<a name="top"></a>

## Contents
  - [UpstreamSpec](#gloo.api.consul.v1.UpstreamSpec)
  - [Connect](#gloo.api.consul.v1.Connect)



<a name="github.com/solo-io/gloo/pkg/plugins/consul/spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.consul.v1.UpstreamSpec"></a>

### UpstreamSpec
Upstream Spec for Consul Upstreams
Consul Upstreams represent a set of one or more instances of a Service that has been registered with Consul
Consul Upstreams map to multiple service instances by the name and tags found on each instance
Consul Upstreams are typically generated automatically by Gloo from the Consul Service Catalog


```yaml
service_name: string
service_tags: [string]
connect: {Connect}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| service_name | string |  | The name of the Service as it is registered in Consul |
| service_tags | string | repeated | The list of service tags Gloo should search for on a service instance before deciding whether or not to include the instance as part of this upstream |
| connect | [Connect](github.com/solo-io/gloo/pkg/plugins/consul/spec.md#gloo.api.consul.v1.Connect) |  | Connect specifies configuration for consul services that are &#34;Connect-enabled&#34;. See for more information about Consul Connect |






<a name="gloo.api.consul.v1.Connect"></a>

### Connect
Connect contains the information necessary to connect to proxies that are running as sidecars for
Consul Connect (in-mesh) services


```yaml
tls_secret_ref: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tls_secret_ref | string |  | A reference to a Gloo secret containing the client TLS parameters for connecting to this service |





 

 

 

