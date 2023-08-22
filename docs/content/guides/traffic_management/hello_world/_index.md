---
title: Hello World
weight: 10
description: Follow this guide for hands on, step-by-step tutorial for creating your first Virtual Service and routing rules in Kubernetes.
---

In this guide, we will introduce Gloo Edge's *Upstream* and *Virtual Service* concepts by accomplishing the following tasks:

* Deploy a REST service to Kubernetes using the Pet Store sample application
* Observe that Gloo Edge's Discovery system finds the Pet Store service and creates an Upstream Custom Resource (CR) for it
* Create a Virtual Service and add routes sending traffic to specific paths on the Pet Store Upstream based on incoming web requests
* Verify Gloo Edge correctly configures Envoy to route to the Upstream
* Test the routes by submitting web requests using `curl`

{{% notice note %}}
If there are no routes configured, Envoy will not be listening on the gateway port.
{{% /notice %}}

---

## Preparing the Environment

To follow along in this guide, you will need to fulfill a few prerequisites.  

### Prerequisite Software

Your local system should have `kubectl` and `glooctl` installed, and you should have access to a Kubernetes deployment to install Gloo Edge.

* [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
* `glooctl`
* Kubernetes v1.11.3+ deployed somewhere. [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) is a
great way to get a cluster up quickly.

### Install Gloo Edge and glooctl

The [linked guide]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes" >}}) walks you through the process of installing `glooctl` locally and installing Gloo Edge on Kubernetes to the default `gloo-system` namespace.

Once you have completed the installation of `glooctl` and Gloo Edge, you are now ready to deploy an example application and configure routing.

--- 

## Example Application Setup

On your Kubernetes installation, you will deploy the Pet Store Application and validate this it is operational.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/helloworld_deploy.mp4" type="video/mp4">
</video>

### Deploy the Pet Store Application

Let's deploy the Pet Store Application on Kubernetes using a YAML file hosted on GitHub. The deployment will stand up the Pet Store container and expose the Pet Store API through a Kubernetes service.

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

```console
deployment.extensions/petstore created
service/petstore created
```

### Verify the Pet Store Application

Now let's verify the pod running the Pet Store application launched successfully the petstore service has been created:

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

### Verify the Upstream for the Pet Store Application

The Gloo Edge discovery services watch for new services added to the Kubernetes cluster. When the petstore service was created, Gloo Edge automatically created an Upstream for the petstore service. If everything deployed properly, the Upstream **STATUS** should be **Accepted**. 

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/helloworld_upstreams.mp4" type="video/mp4">
</video>

Let’s verify this by using the `glooctl` command line tool:

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

This command lists all the Upstreams Gloo Edge has discovered, each written to an *Upstream* CR. 

The Upstream we want to see is `default-petstore-8080`. 
    
{{% notice note %}}
The Upstream was created in the `gloo-system` namespace rather than `default` because it was created by the discovery service. Upstreams and Virtual Services do not need to live in the `gloo-system` namespace to be processed by Gloo Edge. 
{{% /notice %}}

### Investigate the YAML of the Upstream

You can view more information about the properties of a particular Upstream by specifying the output type as `kube-yaml`.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/helloworld_upstreams_2.mp4" type="video/mp4">
</video>

Let's take a closer look at the upstream that Gloo Edge's Discovery service created:

```shell
glooctl get upstream default-petstore-8080 --output kube-yaml
```
```yaml
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
  reportedBy: gloo
  state: 1
```

By default the upstream created is rather simple. It represents a specific kubernetes service. However, the petstore application is a swagger service. Gloo Edge can discover this swagger spec, but by default Gloo Edge's function discovery features are turned off to improve performance. To enable Function Discovery Service (fds) on our petstore, we need to label the namespace.

```shell
kubectl label namespace default  discovery.solo.io/function_discovery=enabled
```

Now Gloo Edge's function discovery will discover the swagger spec. Fds populated our Upstream with the available rest endpoints it implements.

```shell
glooctl get upstream default-petstore-8080
```

```console
+-----------------------+------------+----------+-------------------------+
|       UPSTREAM        |    TYPE    |  STATUS  |         DETAILS         |
+-----------------------+------------+----------+-------------------------+
| default-petstore-8080 | Kubernetes | Accepted | svc name:      petstore |
|                       |            |          | svc namespace: default  |
|                       |            |          | port:          8080     |
|                       |            |          | REST service:           |
|                       |            |          | functions:              |
|                       |            |          | - addPet                |
|                       |            |          | - deletePet             |
|                       |            |          | - findPetById           |
|                       |            |          | - findPets              |
|                       |            |          |                         |
+-----------------------+------------+----------+-------------------------+
```

```shell script
glooctl get upstream default-petstore-8080 --output kube-yaml
```

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  labels:
    discovered_by: kubernetesplugin
    service: petstore
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
status:
  reportedBy: gloo
  state: 1
```

The application endpoints were discovered by Gloo Edge's Function Discovery (fds) service. This was possible because the petstore application implements OpenAPI (specifically, discovering a Swagger JSON document at `petstore-svc/swagger.json`).

---

## Configuring Routing

We have confirmed that the Pet Store application was deployed successfully and that the Function Discovery service on Gloo Edge automatically added an Upstream entry with all the published application endpoints of the Pet Store application. Now let's configure some routing rules on the default Virtual Service and test them to ensure we get a valid response.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/helloworld_virtualservice.mp4" type="video/mp4">
</video>

### Add a Routing Rule

Even though the Upstream has been created, Gloo Edge will not route traffic to it until we add some routing rules on a Virtual Service. Let’s now use glooctl to create a basic route for this Upstream with the `--prefix-rewrite` flag to rewrite the path on incoming requests to match the path our petstore application expects.

```shell
glooctl add route \
  --path-exact /all-pets \
  --dest-name default-petstore-8080 \
  --prefix-rewrite /api/pets
```

{{% notice note %}}
If using Git Bash on Windows, the above will not work; Git Bash interprets the route parameters as Unix file paths and mangles them. Adding `MSYS_NO_PATHCONV=1` to the start of the above command should allow it to execute correctly.
{{% /notice %}}

We do not specify a specific Virtual Service, so the route is added to the `default` Virtual Service. If a `default` Virtual Service does not exist, `glooctl` will create one.

```console
+-----------------+--------------+---------+------+---------+-----------------+---------------------------+
| VIRTUAL SERVICE | DISPLAY NAME | DOMAINS | SSL  | STATUS  | LISTENERPLUGINS |          ROUTES           |
+-----------------+--------------+---------+------+---------+-----------------+---------------------------+
| default         |              | *       | none | Pending |                 | /all-pets -> gloo-system. |
|                 |              |         |      |         |                 | .default-petstore-8080    |
+-----------------+--------------+---------+------+---------+-----------------+---------------------------+
```

The initial **STATUS** of the petstore Virtual Service will be **Pending**. After a few seconds it should change to **Accepted**. Let’s verify that by retrieving the `default` Virtual Service with `glooctl`.

```shell
glooctl get virtualservice default
```

```console
+-----------------+--------------+---------+------+----------+-----------------+---------------------------+
| VIRTUAL SERVICE | DISPLAY NAME | DOMAINS | SSL  | STATUS   | LISTENERPLUGINS |          ROUTES           |
+-----------------+--------------+---------+------+----------+-----------------+---------------------------+
| default         |              | *       | none | Accepted |                 | /all-pets -> gloo-system. |
|                 |              |         |      |          |                 | .default-petstore-8080    |
+-----------------+--------------+---------+------+----------+-----------------+---------------------------+
```

### Verify Virtual Service Creation

Let's verify that a Virtual Service was created with that route. 

Routes are associated with Virtual Services in Gloo Edge. When we created the route in the previous step, we didn't provide a Virtual Service, so Gloo Edge created a Virtual Service called `default` and added the route. 

With `glooctl`, we can see that the `default` Virtual Service was created with our route:

```shell
glooctl get virtualservice default --output kube-yaml
```

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
  ownerReferences: []
status:
  reportedBy: gateway
  state: Accepted
  subresourceStatuses:
    '*v1.Proxy.gloo-system.gateway-proxy':
      reportedBy: gloo
      state: Accepted
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
  ```
    
When a Virtual Service is created, Gloo Edge immediately updates the proxy configuration. Since the status of this Virtual Service is `Accepted`, we know this route is now active. 

At this point we have a Virtual Service with a routing rule sending traffic on the path `/all-pets` to the Upstream `petstore` at a path of `/api/pets`.

### Test the Route Rule

Let’s test the route rule by retrieving the URL of Gloo Edge, and sending a web request to the `/all-pets` path of the URL using curl:

```shell
curl $(glooctl proxy url)/all-pets
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

The proxy has now been configured to route requests to the `/api/pets` REST endpoint on the Pet Store application in Kubernetes.

---

## Next Steps

Congratulations! You've successfully set up your first routing rule. That's just the tip of the iceberg though. In the next sections, we'll take a closer look at more HTTP routing capabilities, including [customizing the matching rules]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_selection/" %}}), [route destination types]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/" %}}), and [request processing features]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/" %}}).

To learn more about the concepts behind Upstreams and Virtual Services check out the [Concepts]({{% versioned_link_path fromRoot="/introduction/architecture/concepts/" %}}) page.

If you're ready to dive deeper into routing, the next logical step is trying out different matching rules starting with [Path Matching]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_selection/path_matching/" %}}).

