---
title: REST Endpoint
weight: 120
description: Route to REST API endpoints discovered from a Swagger (OpenAPI) specification
---

In this guide we will create a route to a specific REST endpoint.

---

## Setup

Let's configure Gloo Edge to route to a single, static Upstream. In this case, we'll route requests through Gloo Edge to the JSON testing API available at `http://jsonplaceholder.typicode.com/`. 

{{< readfile file="/static/content/setup_notes" markdown="true">}}

If you haven't already deployed Gloo Edge and an example swagger service on Kubernetes, [go back to the Hello World]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) guide and run through it to get the Pet Store application deployed.

---

## Configure function routing

Now that we've seen the traditional routing functionality of Gloo Edge (i.e. API-to-service), let's try doing some function routing.

Let's take a look at the Upstream that was created for our petstore service:

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

We can see there are functions on our `default-petstore-8080` Upstream. These functions were populated automatically by the `discovery` pod. You can see the function discovery service in action by running `kubectl logs -l gloo=discovery -n gloo-system`.

The {{< protobuf name="gloo.solo.io.Upstream" display="function spec" >}} you see on the functions listed above is populated by the transformation plugin. This powerful plugin configures Gloo Edge's [request/response transformation Envoy filter](https://github.com/solo-io/envoy-transformation), transforming requests to the structure expected by our Pet Store application.

In a nutshell, this plugin takes [Inja templates](https://github.com/pantor/inja) for HTTP body, headers, and path as its parameters (documented in the plugin spec) and transforms incoming requests from those templates. Parameters for these templates can come from the request body (if it's JSON), or they can come from parameters specified in the extensions on a route.

Let's see how this plugin works by creating some routes to these functions in the next section.

### Create the route

Start by creating the route with `glooctl`:

```shell
glooctl add route \
  --path-exact /petstore/findPet \
  --dest-name default-petstore-8080 \
  --rest-function-name findPetById
```

Notice that, unlike the hello world tutorial, we're passing an extra argument to `glooctl --rest-function-name findPetById`.

### Test the route

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

Try the request again, but now add a JSON body which includes the `id` parameter:

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

Great! We just called our first function through Gloo Edge.

### Pass parameters in a header

Parameters can also come from headers. Let's tell Gloo Edge to look for `id` in a header.

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

We can tell Gloo Edge to grab the template parameters from the request with a flag called `rest-parameters` like this:

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

You may be asking, "Why are you calling that a header, it's not a header"? We're actually calling the service with a path parameter, but in HTTP2 a header called `:path` is used to pass the path information around. At the moment, since Envoy has [built everything internally around HTTP2](https://www.envoyproxy.io/docs/envoy/v1.11.0/intro/arch_overview/http/http_connection_management), we can use this `:path` header to pull template parameters. We could have used another header like `x-gloo` to pass in and then create our `rest-parameters` with the `x-gloo` header and accomplish the same thing. We'll leave that as an exercise to the reader.

---

## Next Steps

In this guide you saw how to use function routing for a REST endpoint. You can learn more about routing and matchers in our guides about [destination selection]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_selection/" %}}).
