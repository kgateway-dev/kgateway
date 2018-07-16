<a name="top"></a>

## Contents
  - [UpstreamSpec](#gloo.api.google.v1.UpstreamSpec)
  - [FunctionSpec](#gloo.api.google.v1.FunctionSpec)



<a name="github.com/solo-io/gloo/pkg/plugins/google/spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.google.v1.UpstreamSpec"></a>

### UpstreamSpec
Upstream Spec for Google Functions Upstreams
AWS Upstreams represent a collection of Google Functions for a particular Google Cloud Platform Account
in a particular region, for a particlar project


```yaml
region: string
project_id: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| region | string |  | The Google Cloud Region in which to run Google Functions |
| project_id | string |  | The Google Cloud Platform project id where the functions are contained |






<a name="gloo.api.google.v1.FunctionSpec"></a>

### FunctionSpec
Function Spec for Functions on Google Functions Upstreams
The Function Spec contains data necessary for Gloo to invoke Google Cloud Platform Functions


```yaml
url: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | string |  | URL is the HTTP Trigger URL assigned to the function in the Google Cloud Functions UI |





 

 

 

