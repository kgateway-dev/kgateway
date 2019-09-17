---
title: "glooctl proxy logs"
weight: 5
---
## glooctl proxy logs

dump Envoy logs from one of the proxy instancesNote: this will enable verbose logging on Envoy

### Synopsis

dump Envoy logs from one of the proxy instancesNote: this will enable verbose logging on Envoy

```
glooctl proxy logs [flags]
```

### Options

```
  -d, --debug    enable debug logging on the proxy as part of this command (default true)
  -f, --follow   enable debug logging on the proxy as part of this command
  -h, --help     help for logs
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

