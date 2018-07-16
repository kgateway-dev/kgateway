<a name="top"></a>

## Contents
  - [UpstreamSpec](#gloo.api.aws.v1.UpstreamSpec)
  - [FunctionSpec](#gloo.api.aws.v1.FunctionSpec)



<a name="github.com/solo-io/gloo/pkg/plugins/aws/spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.aws.v1.UpstreamSpec"></a>

### UpstreamSpec
Upstream Spec for AWS Lambda Upstreams
AWS Upstreams represent a collection of Lambda Functions for a particular AWS Account (IAM Role or User account)
in a particular region


```yaml
region: string
secret_ref: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| region | string |  | The AWS Region in which to run Lambda Functions |
| secret_ref | string |  | A [Gloo Secret Ref](https://gloo.solo.io/introduction/concepts/#Secrets) to an AWS Secret AWS Secrets can be created with `glooctl secret create aws ...` If the secret is created manually, it must conform to the following structure: ``` access_key: &lt;aws access key&gt; secret_key: &lt;aws secret key&gt; ``` |






<a name="gloo.api.aws.v1.FunctionSpec"></a>

### FunctionSpec
Function Spec for Functions on AWS Lambda Upstreams
The Function Spec contains data necessary for Gloo to invoke Lambda functions


```yaml
function_name: string
qualifier: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| function_name | string |  | The Name of the Lambda Function as it appears in the AWS Lambda Portal |
| qualifier | string |  | The Qualifier for the Lambda Function. Qualifiers act as a kind of version for Lambda Functions. See https://docs.aws.amazon.com/lambda/latest/dg/API_Invoke.html for more info. |





 

 

 

