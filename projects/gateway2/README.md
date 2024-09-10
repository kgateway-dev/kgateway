# Gloo Gateway

Note, all commands should be run from the root of the gloo repo.

## APIs

The Kubernetes Gateway API integration is defined in the [api](./api) directory. For more information on how to add a new API or CRD, see the [API README](./api/README.md).

## Quickstart

### Prerequisites

- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/)
- [helm](https://helm.sh/docs/intro/install/)

### Setup

To create the local test environment in kind, run:

```shell
./ci/kind/setup-kind.sh
```

This will create the kind cluster and build the docker images.

Next use helm to install the gateway control plane:

```shell
helm upgrade -i -n gloo-system gloo ./_test/gloo-1.0.0-ci1.tgz --create-namespace --set kubeGateway.enabled=true
```

To create a gateway, use the Gateway resource:

```shell
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: http
spec:
  gatewayClassName: gloo-gateway
  listeners:
  - allowedRoutes:
      namespaces:
        from: All
    name: http
    port: 8080
    protocol: HTTP
EOF
```

Apply a test application such as bookinfo:

```shell
kubectl create namespace bookinfo
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/bookinfo/platform/kube/bookinfo.yaml -n bookinfo
```

Then create a corresponding HTTPRoute:

```shell
kubectl apply -f- <<EOF
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: productpage
  namespace: bookinfo
  labels:
    example: productpage-route
spec:
  parentRefs:
    - name: http
      namespace: default
  hostnames:
    - "www.example.com"
  rules:
    - backendRefs:
        - name: productpage
          port: 9080
EOF
```

### Testing

Expose the gateway that gets created via the Gateway resource:

```shell
kubectl port-forward deployment/gloo-proxy-http 8080:8080
```

Send some traffic through the gateway:

```shell
curl -I localhost:8080/productpage -H "host: www.example.com" -v
```

## Istio Integration

This will create the kind cluster, build the docker images.

```shell
./ci/kind/setup-kind.sh
```

Next we need to install Istio in the cluster along with the bookinfo test application in the mesh:

```shell
./projects/gateway2/istio.sh
```

Next use helm to install the gateway control plane with istio integration enabled:

```shell
helm upgrade -i -n gloo-system gloo ./_test/gloo-1.0.0-ci1.tgz --create-namespace --set kubeGateway.enabled=true --set global.istioSDS.enabled=true
```

In order to enable automtls, set it to true in the settings:

```shell
settings:
  istioOptions:
    enableAutoMtls: true
```

Then expose the gateway that gets created via the Gateway resource:

```shell
kubectl port-forward deployment/gloo-proxy-http 8080:8080
```

Send some traffic through the gateway:

```shell
curl -I localhost:8080/productpage -H "host: www.example.com" -v
```

Test sending traffic to an application not in mtls STRICT mode:

```shell
kubectl apply -f- <<EOF
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: reviews
  namespace: bookinfo
  labels:
    example: reviews-route
spec:
  parentRefs:
    - name: http
      namespace: default
  hostnames:
    - "reviews"
  rules:
    - backendRefs:
        - name: reviews
          port: 9080
EOF
```

Then send traffic to reviews:

```shell
curl -I localhost:8080/reviews/1 -H "host: reviews" -v
```

Test sending traffic to an application not in the mesh:

```shell
# Create non-mesh app (helloworld namespace is not labeled for istio injection)
kubectl create namespace helloworld
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/helloworld/helloworld.yaml -n helloworld
```

Apply an HTTPRoute for helloworld:

```shell
kubectl apply -f- <<EOF
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: helloworld
  namespace: helloworld
  labels:
    example: helloworld-route
spec:
  parentRefs:
    - name: http
      namespace: default
  hostnames:
    - "helloworld"
  rules:
    - backendRefs:
        - name: helloworld
          port: 5000
EOF
```

Send traffic to the non-mesh app:

```shell
curl -I localhost:8080/hello -H "host: helloworld" -v
```
