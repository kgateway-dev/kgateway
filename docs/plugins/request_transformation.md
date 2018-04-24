# Request Transformation Plugin for Gloo


#### Description

The [Transformation Plugin for Gloo](https://github.com/solo-io/gloo-plugins/tree/master/transformation) is the foundation of
HTTP-based function routing. The transformation plugin enables transformation of requests and responses through the use of
templates.

Users of Gloo (including discovery services) can provide templates and parameter sources (used for filling templates)
in order to transform client requests to the structure expected by upstream APIs, as well as transform responses to
the format expected by the client. 

**Request transformations** live on the *function level*. Request transformations can be defined for functions on `service`
and `kubernetes` upstream types. 

The actual content of the transformation is specified using templates. We use [Inja](https://github.com/pantor/inja) 
as our template engine in our [transformation filter](https://github.com/solo-io/envoy-transformation), so the syntax
for the templates used in request transformation should match that specified in the Inja documentation. 

If a function's request template uses named variables (denoted by a string such as `{{ variable_name }}`), Gloo requires that
the routes pointing to that function specify sources for those parameters on the incoming request. The sources for the parameters
live in the route's [`extensions`](../v1/virtualservice.md#Route). A request that fails to supply a parameter to a function
upstream will receive a 400 Bad Request as a response.

If the request body is JSON, the fields in that object can be used as sources for parameters without having to specify 
the body as a source on the route extensions. The template can retrieve variables from the request body by referring to them
using [JSONPath](http://goessner.net/articles/JsonPath/) syntax.  

#### Function Spec Configuration - Request Transformation

Request transformation functions can be added to any supported upstream (currently `kubernetes` and `service` upstreams
are supported).

The [function spec](../v1/upstream.md#Function) for Request Transformation has the following structure:

```yaml
path: <Inja template string>
headers: map<<Inja template string>, <Inja template string>>
body: <Inja template string>
```

| Field | Type |  Description |
| ----- | ---- |  ----------- |
| path | template string | If specified, the outgoing request path will be transformed to this value. |
| headers | map<template string, template string\> | If specified, the outgoing request headers will be transformed to these values. |
| body | template string | The outgoing request body will be transformed to this value. If left empty, the outgoing request body will be empty|

An example request transformation function looks like the following:

```yaml
- name: addPet
  spec:
    body: '{"tag": "{{tag}}","id": {{id}},"name": "{{name}}"}'
    headers:
      :method: POST
    path: /api/pets
``` 


#### Route Spec Configuration - Request Transformation (Parameters)

Request Transformation parameters live on the [`route extensions`](../v1/virtualservice.md#Route), and have the following structure:

```yaml
parameters:
  path: string
  headers: map<string, string>
```
| Field | Type |  Description |
| ----- | ---- |  ----------- |
| parameters | Parameters | the object that contains the parameter sources |
| headers | map<string, string\> | Gloo will search the specified headers for parameters. Parameters should be specified by name in single-curly braces |
| path | string | Gloo will search the path for parameters. Parameters should be specified by name in single-curly braces |

An example route to specify parameters for the above function can look like the following:

```yaml
- request_matcher:
    path_exact: /petstore/add
  single_destination:
    function:
      function_name: addPet
      upstream_name: default-petstore-8080
  extensions:
    parameters:
      headers:
        x-pet-id: '{id}'
        x-pet-tag: '{tag}'
        x-pet-name: '{name}'
```

Gloo will extract the variables named `id`, `tag`, and `name` from the specified headers on the incoming request 
and use them to generate the outgoing request. If they cannot be found in the request headers, Envoy will try to parse
the request body as JSON and search there by json path. If the parameters still cannot be found, Envoy will return a 400
Bad Request to the client.


#### Discovery

The Gloo Function Discovery Service<!--(TODO)--> will automatically discover function transformations for upstreams that
serve a `swagger.json` file. See the [petstore example](../getting_started/kubernetes/1.md) to see an example of 
this discovery service in action.