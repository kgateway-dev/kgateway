# kgateway-waypoint

kgateway is compatible with Istio service mesh! In ambient, L7 processing for
east-west traffic is performed by an independent proxy called a
[waypoint](https://istio.io/latest/docs/ambient/usage/waypoint/). Istio ships with its
own waypoint proxy implementation, and kgateway provides a drop-in replacement.

## Pre-requisites

1. Istio 1.24 or newer with ambient mode
2. kgateway installation

## Install this demo

1. Deploy the test namespace

```bash
kubectl apply -f examples/waypoint/httpbin-mesh.yaml
```

This namespace has two important labels:

```yaml
# ztunnel will capture traffic for all services in this namespace
istio.io/dataplane-mode: ambient

# ztunnel will send traffic to the kgateway-waypoint
istio.io/use-waypoint: httpbin-waypoint
```

2. Deploy the `kgateway-waypoint` Gateway resource

```bash
kubectl apply -f examples/waypoint/waypoint-gw.yaml
```

This Gateway uses GatewayClass `kgateway-waypoint`, which is configured to handle
east-west traffic for your mesh.

3. Apply policy to a Service

```bash
kubectl apply -f examples/waypoint/waypoint-http-route.yaml
```

This `HTTPRoute` is a bit different; it has a parentRef of a `Service` rather
than a `Gateway`. This allows fine-grained policy attachment for in-mesh traffic.
For more info, see the [GAMMA Initiative](https://gateway-api.sigs.k8s.io/mesh/gamma/) documentation.

4. Send some traffic

```bash
CLIENT=$(kubectl get po -n httpbin -l app=curl -ojsonpath='{.items[0].metadata.name}')
kubectl -n httpbin exec $CLIENT -- curl -sS -v httpbin:8000/get
```

You should see the header `Traversed-Waypoint` being added to the response,
indicating the waypoint is now on the data path for all services in the `httpbin`
namespace.

## Istio Authorization Rules Demo

Istio Authorization rules can be applied to the workloads that configured to use waypoint. In
this section it's demonstrated on how to apply and test a simple Authorization.

1. Apply the follow authorization policy that denies GET requests to httpbin service:

```bash
kubectl apply -f examples/waypoint/httpbin-authz.yaml
```

2. Test the traffic follow:

Retrieve the client pod name as above:

```bash
CLIENT=$(kubectl get po -n httpbin -l app=curl -ojsonpath='{.items[0].metadata.name}')
```

- Test `GET` (that is blocked by the rule):

  ```bash
  kubectl -n httpbin exec $CLIENT -- curl -si httpbin:8000/get
  ```

  The request is blocked by the waypoint:

  ```output
  HTTP/1.1 403 Forbidden
  traversed-waypoint: httpbin-waypoint
  content-length: 19
  content-type: text/plain
  date: <...omitted...>
  server: envoy
  
  RBAC: access denied
  ```

- When trying `POST` it is allowed:

  ```bash
  kubectl -n httpbin exec $CLIENT -- curl -sI -XPOST httpbin:8000/post
  ```

  Output shows the successful transaction:

  ```output
  HTTP/1.1 200 OK
  access-control-allow-credentials: true
  access-control-allow-origin: *
  content-type: application/json; charset=utf-8
  date: <...omitted...>
  content-length: 639
  x-envoy-upstream-service-time: 2
  traversed-waypoint: httpbin-waypoint
  server: envoy
  ```

## Deploy on your own

1. Enable ambient traffic capture in the namespace of the Services
   you wish to have routed through the waypoint proxy

```bash
kubectl label namespace httpbin istio.io/dataplane-mode: ambient
```
