---
title: REST Endpoint
weight: 20
description: Create function routes to REST API endpoints discovered from a Swagger (OpenAPI) specification.
---

Let's create a route to a specific REST endpoint. 

## Setup

Let's configure Gloo to route to a single, static upstream. In this case, we'll route requests through Gloo 
to the JSON testing API available at `http://jsonplaceholder.typicode.com/`. 

{{< readfile file="/static/content/setup_notes" markdown="true">}}

## 

If you haven't already deployed Gloo and the example swagger service on kubernetes, [go back to the first tutorial](../../../../../../hello_world) 

Now that we've seen the traditional routing functionality of Gloo (i.e. API-to-service), let's try doing some function routing.

Let's take a look at the upstream that was created for our petstore service:

```shell
glooctl get upstream default-petstore-8080 --output yaml
```
```yaml
...
    serviceSpec:
      rest:
        swaggerInfo:
          url: http://petstore.default.svc.cluster.local:8080/swagger.json
        transformations:
          addPet:
            body:
              text: '{"id": {{ default(id, "") }},"name": "{{ default(name, "")}}","tag":
                "{{ default(tag, "")}}"}'
            headers:
              :method:
                text: POST
              :path:
                text: /api/pets
              content-type:
                text: application/json
          deletePet:
            headers:
              :method:
                text: DELETE
              :path:
                text: /api/pets/{{ default(id, "") }}
              content-type:
                text: application/json
          findPetById:
            body: {}
            headers:
              :method:
                text: GET
              :path:
                text: /api/pets/{{ default(id, "") }}
              content-length:
                text: "0"
              content-type: {}
              transfer-encoding: {}
          findPets:
            body: {}
            headers:
              :method:
                text: GET
              :path:
                text: /api/pets?tags={{default(tags, "")}}&limit={{default(limit,
                  "")}}
              content-length:
                text: "0"
              content-type: {}
              transfer-encoding: {}
...
```

We can see there are functions on our `default-petstore-8080` upstream. These functions were populated automatically by
the `discovery` pod. You can see the function discovery service in action by running `kubectl logs -l gloo=discovery`.

The {{< protobuf name="gloo.solo.io.Upstream" display="function spec" >}} you see on the functions
listed above belongs to the transformation plugin. <!--(TODO)-->

This powerful plugin configures Gloo's [request/response transformation Envoy filter](https://github.com/solo-io/envoy-transformation)
to perform transform requests to the structure expected by our petstore app.

In a nutshell, this plugin takes [Inja templates](https://github.com/pantor/inja) for HTTP body, headers, and path as its parameters
(documented in the plugin spec) and transforms incoming requests from those templates. Parameters for these templates
can come from the request body (if it's JSON), or they can come from parameters specified in the extensions on a route.

Let's see how this plugin works by creating some routes to these functions in the next section.

### Steps

1. Start by creating the route with `glooctl`:

    ```shell
    glooctl add route \
      --path-exact /petstore/findPet \
      --dest-name default-petstore-8080 \
      --rest-function-name findPetById
    ```

    Notice that, unlike the previous tutorial, we're passing an extra argument to `glooctl --rest-function-name findPetById`.

    Let's go ahead and test the route using `curl`:

    ```shell
    curl $(glooctl proxy url)/petstore/findPet
    ```

    ```json
    [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
    ```

    Looking again at the function `findPetById`, you'll notice the template wants a variable called `id`:

    {{< highlight yaml >}}
- name: findPetById
  spec:
    body: ""
    headers:
      :method: GET
    path: /api/pets/{{id}}
    {{< /highlight >}}

1. Try the request again, but now add a JSON body which includes the `id` parameter:

    ```shell
    curl $(glooctl proxy url)/petstore/findPet -d '{"id": 1}'
    ```

    ```json
    {"id":1,"name":"Dog","status":"available"}
    ```

    ```shell
    curl $(glooctl proxy url)/petstore/findPet -d '{"id": 2}'
    ```

    ```json
    {"id":2,"name":"Cat","status":"pending"}
    ```

    Great! We just called our first function through Gloo.

1. Parameters can also come from headers. Let's tell Gloo to look for `id` in a header.

    Let's take a look at the route we created:

    ```shell
    glooctl get virtualservice --output yaml
    ```

    {{< highlight yaml >}}
---
metadata:
  name: default
  namespace: gloo-system
  resourceVersion: "33083"
status:
  reportedBy: gateway
  state: Accepted
  subresourceStatuses:
    '*v1.Proxy gloo-system gateway-proxy':
      reportedBy: gloo
      state: Accepted
virtualHost:
  domains:
  - '*'
  routes:
  - matchers:
     - exact: /petstore/findPet
    routeAction:
      single:
        destinationSpec:
          rest:
            functionName: findPetById
            parameters: {}
        upstream:
          name: default-petstore-8080
          namespace: gloo-system
    {{< /highlight >}}

    We can tell Gloo to grab the template parameters from the request with a flag called `rest-parameters` like this:

    ```shell
    glooctl add route \
      --path-prefix /petstore/findWithId/ \
      --dest-name default-petstore-8080 \
      --rest-function-name findPetById \
      --rest-parameters ':path=/petstore/findWithId/{id}'
    ```

    Try `curl` again, this time with the new header:

    ```shell
    curl $(glooctl proxy url)/petstore/findWithId/1
    ```

    ```json
    {"id":1,"name":"Dog","status":"available"}
    ```

    You may be asking "why are you calling that a header, it's not a header"? We're actually calling the service
    with a path parameter, but in HTTP2 a header called `:path` is used to pass the path information around. At the
    moment, since Envoy has [built everything internally around HTTP2](https://www.envoyproxy.io/docs/envoy/v1.11.0/intro/arch_overview/http/http_connection_management),
    we can use this `:path` header to pull template parameters. We could have used another header like `x-gloo` to pass
    in and then create our `rest-parameters` with the `x-gloo` header and accomplish the same thing. We'll leave that
    as an exercise to the reader.
