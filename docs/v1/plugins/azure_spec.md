<a name="top"></a>

## Contents
  - [UpstreamSpec](#gloo.api.azure.v1.UpstreamSpec)
  - [FunctionSpec](#gloo.api.azure.v1.FunctionSpec)



<a name="github.com/solo-io/gloo/pkg/plugins/azure/spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.azure.v1.UpstreamSpec"></a>

### UpstreamSpec
Upstream Spec for Azure Functions Upstreams
Azure Upstreams represent a collection of Azure Functions for a particular Azure Account (IAM Role or User account)
within a particular Function App


```yaml
function_app_name: string
secret_ref: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| function_app_name | string |  | The Name of the Azure Function App where the functions are grouped |
| secret_ref | string |  | A [Gloo Secret Ref](https://gloo.solo.io/introduction/concepts/#Secrets) to an [Azure Publish Profile JSON file](https://azure.microsoft.com/en-us/downloads/publishing-profile-overview/). {{ hide_not_implemented &#34;Azure Secrets can be created with `glooctl secret create azure ...`&#34; }} Note that this secret is not required unless Function Discovery is enabled |






<a name="gloo.api.azure.v1.FunctionSpec"></a>

### FunctionSpec
Function Spec for Functions on Azure Functions Upstreams
The Function Spec contains data necessary for Gloo to invoke Azure functions


```yaml
function_name: string
auth_level: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| function_name | string |  | The Name of the Azure Function as it appears in the Azure Functions Portal |
| auth_level | string |  | Auth Level can bve either &#34;anonymous&#34; &#34;function&#34; or &#34;admin&#34; See https://vincentlauzon.com/2017/12/04/azure-functions-http-authorization-levels/ for more details |





 

 

 

