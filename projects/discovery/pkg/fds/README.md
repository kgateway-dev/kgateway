# Enabling/Disabling Function Discovery in Kubernetes

Gloo Edge's Function Discovery Service (FDS) runs by default on all namespaces 
except `kube-system` and `kube-public`.

FDS sends http requests to discover well-known OpenAPI Endpoints (e.g.
`/swagger.json`) as well as services implementing gRPC Reflection.

This behavior can be disabled at the namespace or service scope.

Simply run the following command to label a namespace or service to indicate that the services in the namespace (or the service itself) should not be polled by FDS for possible function types:

```bash
# disable fds for all services in a namespace
kubectl label namespace <namespace> discovery.solo.io/function_discovery=disabled
```

```bash
# disable fds for a single service 
kubectl label service -n <namespace> <service> discovery.solo.io/function_discovery=disabled
```

Note that FDS is disabled by default for `kube-system` and `kube-public` namespaces. To enable:

```bash
# enable fds on kube-system
kubectl label namespace kube-system discovery.solo.io/function_discovery=enabled
```