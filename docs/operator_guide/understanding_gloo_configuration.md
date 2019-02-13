---
title: "Understanding Gloo Configuration"
weight: 1
---

---
**NOTE**

Gloo currently requires a running Kubernetes cluster to use as data store. We are adding additional storage options in 
an upcoming release. For a quick test run of Gloo, you can deploy Gloo on `minikube`.

To be notified of the most recent updates, follow us on [Twitter](https://twitter.com/soloio_inc) and join our 
[community Slack channel](http://slack.solo.io/).

---

## Configuration storage
By default, Gloo leverages Kubernetes to implement its 
[declarative infrastructure model](gloo_declarative_model.md#gloo-as-declarative-infrastructure).
The Gloo configuration consists of a set of YAML documents that are stored in Kubernetes as
[custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources).
Each of the Gloo configuration objects (virtual services, upstreams, etc.) is an instance of a 
[Kubernetes Custom Resource Definition (CRD)](https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions).

Storing configuration as Kubernetes CRDs has several advantages:
1. **All Gloo resources can be managed using standard Kubernetes APIs and tools.** You can interact with them using
`kubectl` just as you would with normal Kubernetes resources. Moreover, you can perform the same actions programmatically 
via the [Kubernetes REST API](https://kubernetes.io/docs/reference/using-api/api-overview/) and the provided 
[Client Libraries](https://kubernetes.io/docs/reference/using-api/client-libraries/) built on top of it.
2. **Your Gloo configuration lives close to the resources it applies to.** You don't need to run any additional software. 
Your services and the configurations that define how traffic is routed to them share the same infrastructure and APIs. 
Since Gloo resources are defined as their own resource class they are completely decoupled and isolated from one another. 
You don't have to worry about changes in your Kubernetes resources affecting your Gloo CRDs.
3. **You can leverage existing Kubernetes features to build additional functionality around Gloo.** Want to limit write 
access to the Gloo configuration to sysadmins but give everyone else read access? Just use the Kubernetes RBAC API to 
define the correspondent roles and permissions, like you would with any other Kubernetes resource.

Let's look at some concrete examples that illustrate the relationship between Gloo resources and Kubernetes CRDs.

### List Gloo resources with `kubectl`
In the [Basic Routing chapter of the Getting Started guide](../getting_started/basic_routing.md)
we created a _Virtual Service_ containing one route to the `default-petstore-8080` _Upstream_ by submitting the following 
command:

```bash
glooctl add route \
  --path-exact /sample-route-1 \
  --dest-name default-petstore-8080 \
  --prefix-rewrite /api/pets
```

We can retrieve the _Virtual Service_ specification by running:

```bash
glooctl get virtualservices default -o yaml
```

```yaml
metadata:
  name: default
  namespace: gloo-system
  resourceVersion: "1917"
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
  name: gloo-system.default
  routes:
  - matcher:
      exact: /sample-route-1
    routeAction:
      single:
        upstream:
          name: default-petstore-8080
          namespace: gloo-system
    routePlugins:
      prefixRewrite:
        prefixRewrite: /api/pets
```

The same information can be accessed via `kubectl` by running:

```bash
kubectl get virtualservices.gateway.solo.io/default -n gloo-system -o yaml
```

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: 2019-02-11T20:44:07Z
  generation: 5
  name: default
  namespace: gloo-system
  resourceVersion: "1917"
  selfLink: /apis/gateway.solo.io/v1/namespaces/gloo-system/virtualservices/default
  uid: c6f29633-2e3d-11e9-9ca8-080027dd6d38
spec:
  virtualHost:
    domains:
    - '*'
    name: gloo-system.default
    routes:
    - matcher:
        exact: /sample-route-1
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
      routePlugins:
        prefixRewrite:
          prefixRewrite: /api/pets
status:
  reported_by: gateway
  state: 1
  subresource_statuses:
    '*v1.Proxy gloo-system gateway-proxy':
      reported_by: gloo
      state: 1
```

Note how the above document is a superset of the one returned by `glooctl`.

We are able to access _Virtual Services_ using `kubectl` because Gloo registered a `VirtualService` CRD with the Kubernetes 
API server. You can verify this by running `kubectl get crds`, which should return a list similar to this one:

```bash
NAME                              CREATED AT
gateways.gateway.solo.io          2019-02-11T20:37:04Z
proxies.gloo.solo.io              2019-02-11T20:37:04Z
settings.gloo.solo.io             2019-02-11T20:36:55Z
upstreams.gloo.solo.io            2019-02-11T20:37:01Z
virtualservices.gateway.solo.io   2019-02-11T20:37:04Z
```

You can use the short resource name instead of the fully qualified one in your commands 
(e.g. `kubectl get virtualservice -n gloo-system` or even `kubectl get vs -n gloo-system` instead of 
`kubectl get virtualservices.gateway.solo.io/default -n gloo-system`).

Try to run `kubectl describe crds/virtualservices.gateway.solo.io` to see what a Custom Resource Definition for a Gloo 
resource looks like.

### Writing Gloo resources with `kubectl`
Instead of using `glooctl`, the same _Virtual Service_ could be created using `kubectl`. We first have to create a file 
`~/my-virtual-service.yaml` with the following contents:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    name: gloo-system.default
    routes:
    - matcher:
        exact: /sample-route-1
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
      routePlugins:
        prefixRewrite:
          prefixRewrite: /api/pets
```

We can then use `kubectl apply` to create the resource:

```bash
kubectl apply -f ~/my-virtual-service.yaml
```

You can verify that the _Virtual Service_ and its route have been created by running `kubectl get vs/default -n gloo-system -o yaml`:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: 2019-02-11T20:44:07Z
  generation: 5
  name: default
  namespace: gloo-system
  resourceVersion: "1917"
  selfLink: /apis/gateway.solo.io/v1/namespaces/gloo-system/virtualservices/default
  uid: c6f29633-2e3d-11e9-9ca8-080027dd6d38
spec:
  virtualHost:
    domains:
    - '*'
    name: gloo-system.default
    routes:
    - matcher:
        exact: /sample-route-1
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
      routePlugins:
        prefixRewrite:
          prefixRewrite: /api/pets
status:
  reported_by: gateway
  state: 1
  subresource_statuses:
    '*v1.Proxy gloo-system gateway-proxy':
      reported_by: gloo
      state: 1
```

### Using RBAC to regulate access to Gloo resources
Let's say that we want to restrict access to _Virtual Services_. Only the user `Alice` should have write access, while users 
in the `Developers` group will be able to perform only read operations. Assuming we have the correct authenticator 
modules configured and enabled in our Kubernetes cluster, defining these access rules is as simple as 
`kubectl apply`ing the following YAML documents:

```yaml
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: default
  name: virtualservice-reader
rules:
- apiGroups: ["gateway.solo.io/v1"]
  resources: ["virtualservices"]
  verbs: ["get", "watch", "list"]
```

```yaml
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: read-virtualservices
  namespace: gloo-system
subjects:
- kind: Group
  name: Developers
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: virtualservice-reader
  apiGroup: rbac.authorization.k8s.io
```


```yaml
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: gloo-system
  name: virtualservice-admin
rules:
- apiGroups: ["gateway.solo.io/v1"]
  resources: ["virtualservices"]
  verbs: ["get", "watch", "list", "create", "update", "patch", "delete"]
```

```yaml
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: admin-virtualservices
  namespace: gloo-system
subjects:
- kind: User
  name: Alice
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: virtualservice-admin
  apiGroup: rbac.authorization.k8s.io
```

After these resources have been created, if `Bob`, who belongs to the `Developers` group, tries to create a virtual 
service by running

```bash
kubectl apply -f ~/my-virtual-service.yaml
```

he will be presented with an error message:

```
Error from server (Forbidden): virtualservices is forbidden: User "Bob" cannot create virtualservices in the namespace "gloo-system"
```

Again, this works because Kubernetes treats CRDs as regular resources.