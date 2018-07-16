<a name="top"></a>

## Contents
  - [RouteExtension](#gloo.api.rest.v1.RouteExtension)
  - [Parameters](#gloo.api.rest.v1.Parameters)
  - [TransformationSpec](#gloo.api.rest.v1.TransformationSpec)



<a name="github.com/solo-io/gloo/pkg/plugins/rest/spec"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.rest.v1.RouteExtension"></a>

### RouteExtension
The REST Route Extension contains two components:
* parameters for calling REST functions
* Response Transformation


```yaml
parameters: {Parameters}
response_transformation: {TransformationSpec}
response_params: {Parameters}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| parameters | [Parameters](github.com/solo-io/gloo/pkg/plugins/rest/spec.md#gloo.api.rest.v1.Parameters) |  | If specified, these parameters will be used as inputs for REST templates for the destination function for the route (if the route destination is a functional destination that has a REST transformation) |
| response_transformation | [TransformationSpec](github.com/solo-io/gloo/pkg/plugins/rest/spec.md#gloo.api.rest.v1.TransformationSpec) |  | If specified, responses on this route will be transformed according to the template(s) provided in the transformation spec here |
| response_params | [Parameters](github.com/solo-io/gloo/pkg/plugins/rest/spec.md#gloo.api.rest.v1.Parameters) |  | If specified, paremeters for the response transformation will be extracted from these sources |






<a name="gloo.api.rest.v1.Parameters"></a>

### Parameters
Parameters define a set of parameters for REST Transformations
Parameters can be extracted from HTTP Headers and Request Path
Parameters can also be extracted from the HTTP Body, provided that it is
valid JSON-encoded
Gloo will search for parameters by their name in strings, enclosed in single
curly braces, and attempt to match them to the variables in REST Function Templates
for example:
  # route
  match: {...}
  destination: {...}
  extensions:
    parameters:
        headers:
          x-user-id: { userId }
  ---
  # function
  name: myfunc
  spec:
    body: |
    {
      &#34;id&#34;: {{ userId }}
    }


```yaml
headers: map<string,string>
path: {google.protobuf.StringValue}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| headers | map&lt;string,string&gt; |  | headers that will be used to extract data for processing output templates Gloo will search for parameters by their name in header value strings, enclosed in single curly braces Example: extensions: parameters: headers: x-user-id: { userId } |
| path | [google.protobuf.StringValue](github.com/solo-io/gloo/pkg/plugins/rest/spec.md#google.protobuf.StringValue) |  | part of the (or the entire) path that will be used extract data for processing output templates Gloo will search for parameters by their name in header value strings, enclosed in single curly braces Example: extensions: parameters: path: /users/{ userId } TODO: support query params TODO: support form params |






<a name="gloo.api.rest.v1.TransformationSpec"></a>

### TransformationSpec
TransformationSpec can act as part of a Route Extension (as a Response Transformation), or as
a FunctionSpec (as a Request Transformation).
Use TransformationSpec as the Function Spec for REST Services (where `Upstream.ServiceInfo.Type == &#34;REST&#34;`)
TransformationSpec contains a set of templates that will be used to modify the Path, Headers, and Body
Parameters for the tempalte come from the following sources:
path: HTTP Request path (if present)
method: HTTP Request method (if present)
parameters specified in the RouteExtension.Parameters (or, in the case of ResponseTransformation, RouteExtension.ResponseParams)
Parameters can also be extracted from the Request / Response Body provided that they are JSON
To do so, specify the field using JSONPath syntax
any field from the request body, assuming it&#39;s json (http://goessner.net/articles/JsonPath/index.html#e2)
Note: REST Service detection and configuration can be performed automatically by Gloo for services that
implement Swagger and serve their `swagger.json` file on a common endpoint (e.g. /v1/swagger.json).
Custom endpoints for swagger.json can be added via the Function Discovery configuration. Requires Function Discovery to be enabled.


```yaml
path: string
headers: map<string,string>
body: {google.protobuf.StringValue}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | string |  | a Jinja-style Template string for the outbound request path. Only useful for request transformation |
| headers | map&lt;string,string&gt; |  | a map of keys to Jinja-style Template strings HTTP Headers. Useful for request and response transformations |
| body | [google.protobuf.StringValue](github.com/solo-io/gloo/pkg/plugins/rest/spec.md#google.protobuf.StringValue) |  | a Jinja-style Template string for the outbound HTTP Body. Useful for request and response transformations If this is nil, the body will be passed through unmodified. If set to an empty string, the body will be removed from the HTTP message. TODO: support query template TODO: support form template |





 

 

 

