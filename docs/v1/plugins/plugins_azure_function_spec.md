<a name="top"></a>

## Contents
  - [FunctionSpec](#gloo.api.azure.v1.FunctionSpec)



<a name="github.com/solo-io/gloo/pkg/plugins/azure/function_spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.azure.v1.FunctionSpec"></a>

### FunctionSpec
Function Spec for Functions on Azure Functions Upstreams


```yaml
function_name: string
auth_level: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| function_name | string |  | The Name of the Azure Function as it appears in the Azure Functions Portal |
| auth_level | string |  | Auth Level can bve either &#34;anonymous&#34; &#34;function&#34; or &#34;admin&#34; See https://vincentlauzon.com/2017/12/04/azure-functions-http-authorization-levels/ for more details |





 

 

 

