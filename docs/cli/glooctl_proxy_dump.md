---
title: "glooctl proxy dump"
weight: 5
---
## glooctl proxy dump

dump Envoy config from one of the proxy instances

### Synopsis

dump Envoy config from one of the proxy instances

```
glooctl proxy dump [flags]
```

### Options

```
  -h, --help   help for dump
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

