---
title: Hello World
weight: 10
description: Follow this guide for hands on, step-by-step tutorial for creating your first virtual service and routing rules in Kubernetes.
---

In this guide, we will introduce Gloo's `Upstream` and `Virtual Service` concepts. 
We will deploy a REST service to Kubernetes, and we will see that Gloo's Discovery system found that service
and created an `Upstream` CRD for it, to be used as a destination for routing. We will then 
create a route on a `Virtual Service` to an endpoint on that `Upstream` and verify Gloo 
correctly configures Envoy to route to that endpoint. 

{{% notice note %}}
If there are no routes configured, Envoy will not be listening on the gateway port.
{{% /notice %}}

### What you'll need

* [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
* Kubernetes v1.11.3+ deployed somewhere. [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) is a
great way to get a cluster up quickly.

### Steps

1. Start by [installing]({{< versioned_link_path fromRoot="/installation" >}}) the Gloo Gateway on Kubernetes to the default `gloo-system` namespace.

1. Next, deploy the Pet Store app to kubernetes:

    ```shell
    kubectl apply \
      --filename https://raw.githubusercontent.com/sololabs/demos2/master/resources/petstore.yaml
    ```

1. The discovery services should have already created an Upstream for the petstore service.
Let's verify this:

    ```shell
    glooctl get upstreams
    ```

    ```noop
    +--------------------------------+------------+----------+------------------------------+
    |            UPSTREAM            |    TYPE    |  STATUS  |           DETAILS            |
    +--------------------------------+------------+----------+------------------------------+
    | default-kubernetes-443         | Kubernetes | Pending  | svc name:      kubernetes    |
    |                                |            |          | svc namespace: default       |
    |                                |            |          | port:          8443          |
    |                                |            |          |                              |
    | default-petstore-8080          | Kubernetes | Accepted | svc name:      petstore      |
    |                                |            |          | svc namespace: default       |
    |                                |            |          | port:          8080          |
    |                                |            |          | REST service:                |
    |                                |            |          | functions:                   |
    |                                |            |          | - addPet                     |
    |                                |            |          | - deletePet                  |
    |                                |            |          | - findPetById                |
    |                                |            |          | - findPets                   |
    |                                |            |          |                              |
    | gloo-system-gateway-proxy-8080 | Kubernetes | Accepted | svc name:      gateway-proxy |
    |                                |            |          | svc namespace: gloo-system   |
    |                                |            |          | port:          8080          |
    |                                |            |          |                              |
    | gloo-system-gloo-9977          | Kubernetes | Accepted | svc name:      gloo          |
    |                                |            |          | svc namespace: gloo-system   |
    |                                |            |          | port:          9977          |
    |                                |            |          |                              |
    +--------------------------------+------------+----------+------------------------------+
    ```

    This command lists all the upstreams Gloo has discovered, each written to an `Upstream` CRD. 
    The upstream we want to see is `default-petstore-8080`. 
    Digging a little deeper, we can verify that Gloo's function discovery populated our upstream with
    the available rest endpoints it implements. 
    
{{% notice note %}}
The upstream was created in the `gloo-system` namespace rather than `default` because it was created by a
discovery service. Upstreams and Virtual Services do not need to live in the `gloo-system` namespace to be 
processed by Gloo. 
{{% /notice %}}

1. Let's take a closer look at the upstream that Gloo's Discovery service created:

    ```shell
    glooctl get upstream default-petstore-8080 --output yaml
    ```

    ```yaml
    ---
    discoveryMetadata: {}
    metadata:
      annotations:
        kubectl.kubernetes.io/last-applied-configuration: |
          {"apiVersion":"v1","kind":"Service","metadata":{"annotations":{},"labels":{"service":"petstore"},"name":"petstore","namespace":"default"},"spec":{"ports":[{"port":8080,"protocol":"TCP"}],"selector":{"app":"petstore"}}}
      labels:
        discovered_by: kubernetesplugin
        service: petstore
      name: default-petstore-8080
      namespace: gloo-system
      resourceVersion: "268143"
    status:
      reportedBy: gloo
      state: Accepted
    kube:
      selector:
        app: petstore
      serviceName: petstore
      serviceNamespace: default
      servicePort: 8080
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
    ```

    The application endpoints were discovered by Gloo's Function Discovery (fds) service. This was possible because the petstore
    application implements OpenAPI (specifically, discovering a Swagger JSON document at `petstore-svc/swagger.json`).
    Note that some functions were discovered. We will use these to demonstrate function routing in the [next tutorial](../virtual_services/routes/route_destinations/single_upstreams/function_routing/).

1. Let's now use `glooctl` to create a basic route for this upstream.

    ```shell
    glooctl add route \
        --path-exact /sample-route-1 \
        --dest-name default-petstore-8080 \
        --prefix-rewrite /api/pets
    ```

    We use the `--prefix-rewrite` to rewrite path on incoming requests
    to match the paths our petstore expects.

{{% notice note %}}
See TODO for a guide on creating a route to a REST endpoint without requiring 
prefix rewriting. 
{{% /notice %}}

1. Let's verify that a virtual service was created with that route. 

    Routes are associated with virtual services in Gloo. When we created the route 
    in the previous step, we didn't provide a virtual service, so Gloo created
    a virtual service called `default` and added the route. 

    With `glooctl`, we can see that the default virtual service was created with our route:

    ```shell
    glooctl get virtualservice --output yaml
    ```

    {{< highlight yaml >}}
---
metadata:
  name: default
  namespace: gloo-system
  resourceVersion: "268264"
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
     - exact: /sample-route-1
    routeAction:
      single:
        upstream:
          name: default-petstore-8080
          namespace: gloo-system
    options:
      prefixRewrite: /api/pets
    {{< /highlight >}}
    
When a virtual service is created, Gloo immediately updates the proxy configuration. Since the 
status of this virtual service is `Accepted`, we know this route is now active. 

Let's test the route `/sample-route-1` using `curl`:

```shell
curl $(glooctl proxy url)/sample-route-1
```
returns
```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

The proxy has now been configured to route requests to this REST endpoint in Kubernetes. 

In the next sections, we'll take a closer look at more HTTP routing capabilities, including 
customizing the matching rules, route destinations, other routing features, 
security, and more. We'll also look at TCP routing, advanced proxy configuration, and 
integrations with different applications and service meshes. 

