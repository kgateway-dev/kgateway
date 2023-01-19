---
title: "glooctl istio inject"
weight: 5
---
## glooctl istio inject

Enable SDS & istio-proxy sidecars in gateway-proxy pod

### Synopsis

Adds an istio-proxy sidecar to the gateway-proxy pod for mTLS certificate generation purposes. Also adds an sds sidecar to the gateway-proxy pod for mTLS certificate rotation purposes.

```
glooctl istio inject [flags]
```

### Options

```
  -h, --help                             help for inject
      --istio-discovery-address string   Sets discoveryAddress field within PROXY_CONFIG environment variable
      --istio-meta-cluster-id string     Sets ISTIO_META_CLUSTER_ID environment variable
      --istio-meta-mesh-id string        Sets ISTIO_META_MESH_ID environment variable
      --istio-namespace string           Namespace in which Istio is installed (default "istio-system")
```

### Options inherited from parent commands

```
  -c, --config string              set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --consul-address string      address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-allow-stale-reads   Allows reading using Consul's stale consistency mode.
      --consul-datacenter string   Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string     key prefix for for Consul key-value storage. (default "gloo")
      --consul-scheme string       URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string        Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
  -i, --interactive                use interactive mode
      --kube-context string        kube context to use when interacting with kubernetes
      --kubeconfig string          kubeconfig to use, if not standard one
      --name string                name of the resource to read or write
  -n, --namespace string           namespace for reading or writing resources (default "gloo-system")
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl istio](../glooctl_istio)	 - Commands for interacting with Istio in Gloo

