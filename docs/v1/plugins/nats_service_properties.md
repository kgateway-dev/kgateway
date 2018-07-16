<a name="top"></a>

## Contents
  - [ServiceProperties](#gloo.api.nats.v1.ServiceProperties)



<a name="github.com/solo-io/gloo/pkg/plugins/nats/service_properties"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.nats.v1.ServiceProperties"></a>

### ServiceProperties
Service Properties for NATS-Streaming clusters
Service Properties must be set to enable HTTP-to-NATS
Message transformation via Gloo.


```yaml
cluster_id: string
discover_prefix: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_id | string |  | the cluster ID of the NATS-streaming service defaults to `test-cluster` |
| discover_prefix | string |  | the NATS-streaming discover prefix defaults to `_STAN.discover` |





 

 

 

