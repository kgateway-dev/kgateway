---
title: Hello World
weight: 10
description: Follow this guide for hands on, step-by-step tutorial for creating your first virtual service and routing rules in Kubernetes.
---

In this guide, we will introduce Gloo's *Upstream* and *Virtual Service* concepts. 

We will deploy a REST service to Kubernetes using the Pet Store sample application, and we will see that Gloo's Discovery system finds that service
and creates an *Upstream* Custom Resource Definition (CRD) for it, to be used as a destination for routing. 

Next we will create a *Virtual Service* and add routes sending traffic to specific paths on the Pet Store *Upstream* based on incoming web requests, and verify Gloo correctly configures Envoy to route to that endpoint.

Finally, we will test the routes by submitting web requests using `curl`.

{{% notice note %}}
If there are no routes configured, Envoy will not be listening on the gateway port.
{{% /notice %}}

### What you'll need

* [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
* Kubernetes v1.11.3+ deployed somewhere. [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) is a
great way to get a cluster up quickly.

### Steps

#### Install the Gloo Gateway and glooctl

Start by [installing]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes" >}}) the Gloo Gateway on Kubernetes to the default `gloo-system` namespace. Then install `glooctl` on the same system you will use to run the `kubectl` commands.

#### Deploy the Pet Store Application

Turn on function discovery in the `default` namespace with:

```shell script
kubectl label namespace default discovery.solo.io/function_discovery=enabled
```
Next, deploy the Pet Store app to kubernetes:

```shell
kubectl apply \
  --filename https://raw.githubusercontent.com/solo-io/gloo/feature-rc1/example/petstore/petstore.yaml
```

```console
deployment.apps/petstore created
service/petstore created
```

#### Verify the Pet Store Application

Now let's verify the petstore pod is running and the petstore service has been created:

```shell
kubectl -n default get pods
```
```console
NAME                READY  STATUS   RESTARTS  AGE
petstore-####-####  1/1    Running  0         30s
```
If the pod is not yet running, run the `kubectl -n default get pods -w` command and wait until it is. Then enter `Ctrl-C` to break out of the wait loop.

Let's verify that the petstore service has been created as well.

```shell
kubectl -n default get svc petstore
```

Note that the service does not have an external IP address. It is only accessible within the Kubernetes cluster.

```console
NAME      TYPE       CLUSTER-IP   EXTERNAL-IP  PORT(S)   AGE
petstore  ClusterIP  10.XX.XX.XX  <none>       8080/TCP  1m
```

#### Verify the Upstream for the Pet Store Application

The Gloo discovery services should have already created an upstream for the petstore service, and the **STATUS** should be **Accepted**. Let’s verify this by using the `glooctl` command line tool:

```shell
glooctl get upstreams
```
```console
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

This command lists all the upstreams Gloo has discovered, each written to an *Upstream* CRD. 

The upstream we want to see is `default-petstore-8080`. 

Digging a little deeper, we can verify that Gloo's function discovery populated our upstream with the available rest endpoints it implements. 
    
{{% notice note %}}
The upstream was created in the `gloo-system` namespace rather than `default` because it was created by a discovery service. Upstreams and Virtual Services do not need to live in the `gloo-system` namespace to be processed by Gloo. 
{{% /notice %}}

#### Investigate the YAML of the Upstream

Let's take a closer look at the upstream that Gloo's Discovery service created:

```shell
glooctl get upstream default-petstore-8080 --output kube-yaml
```
```yaml
---
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  labels:
    app: petstore
    discovered_by: kubernetesplugin
  name: default-petstore-8080
  namespace: gloo-system
spec:
  discoveryMetadata: {}
  kube:
    selector:
      app: petstore
    serviceName: petstore
    serviceNamespace: default
    servicePort: 8080
status:
  reported_by: gloo
  state: 1
```

By default the upstream created is rather simple. It represents a specific kubernetes service. However, the petstore application is
a swagger service. Gloo can discover this swagger spec, but by default Gloo's function discovery features are turned off to improve 
performance. To enable Function Discovery Service (fds) on our petstore, we need to label the namespace.
```shell
kubectl label namespace default  discovery.solo.io/function_discovery=enabled
```

Now fds will discovery the swagger spec.

```shell script
glooctl get upstream default-petstore-8080 --output yaml
```
```yaml
---
discoveryMetadata: {}
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
              text: /api/pets?tags={{default(tags, "")}}&limit={{default(limit, "")}}
            content-length:
              text: "0"
            content-type: {}
            transfer-encoding: {}
metadata:
  labels:
    app: petstore
    discovered_by: kubernetesplugin
  name: default-petstore-8080
  namespace: gloo-system
status:
  reportedBy: gloo
  state: Accepted


```

#### Add a Routing Rule

Even though the upstream has been created, Gloo will not route traffic to it until we add some routing rules on a `virtualservice`. Let’s now use glooctl to create a basic route for this upstream with the `--prefix-rewrite` flag to rewrite the path on incoming requests to match the path our petstore application expects.

```shell
glooctl add route \
  --path-exact /all-pets \
  --dest-name default-petstore-8080 \
  --prefix-rewrite /api/pets
```

```console
+-----------------+--------------+---------+------+---------+-----------------+---------------------------+
| VIRTUAL SERVICE | DISPLAY NAME | DOMAINS | SSL  | STATUS  | LISTENERPLUGINS |          ROUTES           |
+-----------------+--------------+---------+------+---------+-----------------+---------------------------+
| petstore        |              | *       | none | Pending |                 | /all-pets -> gloo-system. |
|                 |              |         |      |         |                 | .default-petstore-8080    |
+-----------------+--------------+---------+------+---------+-----------------+---------------------------+
```

The initial **STATUS** of the petstore virtual service will be **Pending**. After a few seconds it should change to **Accepted**. Let’s verify that by retrieving the `virtualservice` with `glooctl`.

```shell
glooctl get virtualservice
```

```console
+-----------------+--------------+---------+------+----------+-----------------+---------------------------+
| VIRTUAL SERVICE | DISPLAY NAME | DOMAINS | SSL  | STATUS   | LISTENERPLUGINS |          ROUTES           |
+-----------------+--------------+---------+------+----------+-----------------+---------------------------+
| default         |              | *       | none | Accepted |                 | /all-pets -> gloo-system. |
|                 |              |         |      |          |                 | .default-petstore-8080    |
+-----------------+--------------+---------+------+----------+-----------------+---------------------------+
```

{{% notice note %}}
See TODO for a guide on creating a route to a REST endpoint without requiring 
prefix rewriting. 
{{% /notice %}}

#### Verify Virtual Service Creation

Let's verify that a virtual service was created with that route. 

Routes are associated with virtual services in Gloo. When we created the route in the previous step, we didn't provide a virtual service, so Gloo created a virtual service called `default` and added the route. 

With `glooctl`, we can see that the default virtual service was created with our route:

```shell
glooctl get virtualservice --output yaml
```

{{< highlight yaml >}}
---
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - exact: /all-pets
      options:
        prefixRewrite: /api/pets
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
status:
  reported_by: gateway
  state: 1
  subresource_statuses:
    '*v1.Proxy.gloo-system.gateway-proxy':
      reported_by: gloo
      state: 1
{{< /highlight >}}
    
When a virtual service is created, Gloo immediately updates the proxy configuration. Since the status of this `virtualservice` is `Accepted`, we know this route is now active. 

At this point we have a `virtualservice` with a routing rule sending traffic on the path `/all-pets` to the `upstream` petstore at a path of `/api/pets`.

#### Test the Route Rule

Let’s test the route rule by retrive the url of the Gloo gateway, and sending a web request to the `/all-pets` path of the url using curl:

```shell
curl $(glooctl proxy url)/all-pets
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

The proxy has now been configured to route requests to this REST endpoint in Kubernetes. 

In the next sections, we'll take a closer look at more HTTP routing capabilities, including customizing the matching rules, route destinations, other routing features, security, and more. We'll also look at TCP routing, advanced proxy configuration, and integrations with different applications and service meshes. 

