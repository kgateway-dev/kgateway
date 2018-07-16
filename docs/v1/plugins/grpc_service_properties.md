<a name="top"></a>

## Contents
  - [ServiceProperties](#gloo.api.grpc.v1.ServiceProperties)



<a name="github.com/solo-io/gloo/pkg/plugins/grpc/service_properties"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.grpc.v1.ServiceProperties"></a>

### ServiceProperties
Service Properties for gRPC Services
Service Properties must be set to enable JSON-to-gRPC Transcoding for gRPC Services
via Gloo.
Note: gRPC detection and configuration can be performed automatically by Gloo for services that
support gRPC Reflection. Function Discovery must be enabled.


```yaml
grpc_service_names: [string]
descriptors_file_ref: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| grpc_service_names | string | repeated | the names of the gRPC services defined in the descriptors. The methods on services specified here can be called using JSON/REST via Gloo&#39;s function-level routing |
| descriptors_file_ref | string |  | The [Gloo File Ref](https://gloo.solo.io/introduction/concepts/#Files) to a File containing the proto descriptors generated for the gRPC service This file will be generated automatically by Gloo Function Discovery if it is enabled and the gRPC service supports Reflection |





 

 

 

