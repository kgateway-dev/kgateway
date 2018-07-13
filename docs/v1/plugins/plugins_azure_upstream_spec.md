<a name="top"></a>

## Contents
  - [UpstreamSpec](#gloo.api.azure.v1.UpstreamSpec)



<a name="github.com/solo-io/gloo/pkg/plugins/azure/upstream_spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.azure.v1.UpstreamSpec"></a>

### UpstreamSpec
Upstream Spec for Azure Functions Upstreams


```yaml
function_app_name: string
secret_ref: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| function_app_name | string |  | The Name of the Azure Function App where the functions are grouped |
| secret_ref | string |  | A [Gloo Secret Ref](https://gloo.solo.io/introduction/concepts/#Secrets) to an [Azure Publish Profile JSON file](https://azure.microsoft.com/en-us/downloads/publishing-profile-overview/). {{ hide_not_implemented &#34;Azure Secrets can be created with `glooctl secret create azure ...`&#34; }} Note that this secret is not required unless Function Discovery is enabled |





 

 

 

