---
title: "glooctl proxy url"
weight: 5
---
## glooctl proxy url

print the http endpoint for a proxy

### Synopsis

Use this command to view the HTTP URL of a Proxy reachable from outside the cluster. You can connect to this address from a host on the same network (such as the host machine, in the case of minikube/minishift).

```
glooctl proxy url [flags]
```

### Options

```
  -h, --help               help for url
  -l, --local-cluster      use when the target kubernetes cluster is running locally, e.g. in minikube or minishift. this will default to true if LoadBalanced services are not assigned external IPs by your cluster
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
  -p, --name string   the name of the proxy service/deployment to use (default "gateway-proxy")
      --port string   the name of the service port to connect to (default "http")
```

### SEE ALSO

* [glooctl proxy](../glooctl_proxy)	 - interact with proxy instances managed by Gloo

