<a name="top"></a>

## Contents
  - [UpstreamSpec](#gloo.api.kubernetes.v1.UpstreamSpec)



<a name="github.com/solo-io/gloo/pkg/plugins/kubernetes/spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.kubernetes.v1.UpstreamSpec"></a>

### UpstreamSpec
Upstream Spec for Kubernetes Upstreams
Kubernetes Upstreams represent a set of one or more addressable pods for a Kubernetes Service
the Gloo Kubernetes Upstream maps to a single service port. Because Kubernetes Services support mulitple ports,
Gloo requires that a different upstream be created for each port
Kubernetes Upstreams are typically generated automatically by Gloo from the Kubernetes API


```yaml
service_name: string
service_namespace: string
service_port: int32
labels: map<string,string>

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| service_name | string |  | The name of the Kubernetes Service |
| service_namespace | string |  | The namespace where the Service lives |
| service_port | int32 |  | The port where the Service is listening. If the service only has one port, this can be left empty |
| labels | map&lt;string,string&gt; |  | Labels allow finer-grained filtering of pods for the Upstream. Gloo will select pods based on their labels if any are provided here. (see [Kubernetes labels and selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/) |





 

 

 

