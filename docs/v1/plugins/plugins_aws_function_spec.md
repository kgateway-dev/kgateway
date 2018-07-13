<a name="top"></a>

## Contents
  - [FunctionSpec](#gloo.api.aws.v1.FunctionSpec)



<a name="github.com/solo-io/gloo/pkg/plugins/aws/function_spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.aws.v1.FunctionSpec"></a>

### FunctionSpec
Function Spec for Functions on AWS Lambda Upstreams


```yaml
function_name: string
qualifier: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| function_name | string |  | The Name of the Lambda Function as it appears in the AWS Lambda Portal |
| qualifier | string |  | The Qualifier for the Lambda Function. Qualifiers act as a kind of version for Lambda Functions. See https://docs.aws.amazon.com/lambda/latest/dg/API_Invoke.html for more info. |





 

 

 

