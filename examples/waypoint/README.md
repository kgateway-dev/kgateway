# kgateway-waypoint

kgateway is compatible with Istio service mesh! In ambient, L7 processing for
east-west traffic is performed by an independent proxy called a
[waypoint](https://istio.io/latest/docs/ambient/usage/waypoint/). Istio ships with its
own waypoint proxy implementation, and kgateway provides a drop-in replacement.


## Pre-requisites

1. Istio 1.23 or newer with ambient mode
2. kgateway installation


## Install this demo

1. Deploy the test namespace

```
kubectl apply -f examples/waypoint/httpbin-mesh.yaml
```

This namespace has two important labels:

```
# ztunnel will capture traffic for all services in this namespace
istio.io/dataplane-mode: ambient

# ztunnel will send traffic to the kgateway-waypoint
istio.io/use-waypoint: httpbin-waypoint
```

2. Deploy the `kgateway-waypoint` Gateway resource

```
kubectl apply -f examples/waypoint/waypoint-gw.yaml
```

This Gateway uses GatewayClass `kgateway-waypoint`, which is configured to handle
east-west traffic for your mesh.

3. Apply policy to a Service

```
kubectl apply -f examples/waypoint/service-http-route.yaml
```

This `HTTPRoute` is a bit different; it has a parentRef of a `Service` rather
than a `Gateway`. This allows fine-grained policy attachment for in-mesh traffic.
For more info, see the [GAMMA Initiative](https://gateway-api.sigs.k8s.io/mesh/gamma/) documentation.

4. Send some traffic

```
CLIENT=$(kubectl get po -n httpbin -ojsonpath='{.items[0].metadata.name}')
kubectl -n httpbin exec $CLIENT -- curl -sS -v httpbin:8000/get
```

You should see the header `Traversed-Waypoint` being added to the response,
indicating the waypoint is now on the data path for all services in the `httpbin`
namespace.

## Deploy on your own

1. Enable ambient traffic capture in the namespace of the Services
   you wish to have routed through the waypoint proxy

```
kubectl label namespace httpbin istio.io/dataplane-mode: ambient
```
