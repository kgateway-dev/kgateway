---
title: "glooctl proxy address"
weight: 5
---
## glooctl proxy address

print the socket address for a proxy

### Synopsis

Use this command to view the address (host:port) of a Proxy reachable from outside the cluster. You can connect to this address from a host on the same network (such as the host machine, in the case of minikube/minishift).

```
glooctl proxy address [flags]
```

### Options

```
  -h, --help                        help for address
  -l, --local-cluster               use when the target kubernetes cluster is running locally, e.g. in minikube or minishift. this will default to true if LoadBalanced services are not assigned external IPs by your cluster
  -p, --local-cluster-name string   name of the locally running minikube cluster. (default "minikube")
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
      --name string                the name of the proxy service/deployment to use (default "gateway-proxy")
  -n, --namespace string           namespace for reading or writing resources (default "gloo-system")
      --port string                the name of the service port to connect to (default "http")
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl proxy](../glooctl_proxy)	 - interact with proxy instances managed by Gloo

