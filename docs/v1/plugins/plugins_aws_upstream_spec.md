<a name="top"></a>

## Contents
  - [UpstreamSpec](#gloo.api.aws.v1.UpstreamSpec)



<a name="github.com/solo-io/gloo/pkg/plugins/aws/upstream_spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.aws.v1.UpstreamSpec"></a>

### UpstreamSpec
Upstream Spec for AWS Lambda Upstreams


```yaml
region: string
secret_ref: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| region | string |  | The AWS Region in which to run Lambda Functions |
| secret_ref | string |  | A [Gloo Secret Ref](https://gloo.solo.io/introduction/concepts/#Secrets) to an AWS Secret AWS Secrets can be created with `glooctl secret create aws ...` If the secret is created manually, it must conform to the following structure: ``` access_key: &lt;aws access key&gt; secret_key: &lt;aws secret key&gt; ``` |





 

 

 

