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
  -i, --interactive         use interactive mode
      --kubeconfig string   kubeconfig to use, if not standard one
      --name string         the name of the proxy service/deployment to use (default "gateway-proxy-v2")
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
      --port string         the name of the service port to connect to (default "http")
```

### SEE ALSO

* [glooctl proxy](../glooctl_proxy)	 - interact with proxy instances managed by Gloo

