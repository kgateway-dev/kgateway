---
title: "glooctl proxy"
weight: 5
---
## glooctl proxy

interact with proxy instances managed by Gloo

### Synopsis

these commands can be used to interact directly with the Proxies Gloo is managing. They are useful for interacting with and debugging the proxies (Envoy instances) directly.

### Options

```
  -h, --help               help for proxy
      --name string        the name of the proxy service/deployment to use (default "gateway-proxy-v2")
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
      --port string        the name of the service port to connect to (default "http")
```

### Options inherited from parent commands

```
  -i, --interactive         use interactive mode
      --kubeconfig string   kubeconfig to use, if not standard one
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo
* [glooctl proxy address](../glooctl_proxy_address)	 - print the socket address for a proxy
* [glooctl proxy dump](../glooctl_proxy_dump)	 - dump Envoy config from one of the proxy instances
* [glooctl proxy logs](../glooctl_proxy_logs)	 - dump Envoy logs from one of the proxy instancesNote: this will enable verbose logging on Envoy
* [glooctl proxy served-config](../glooctl_proxy_served-config)	 - dump Envoy config being served by the Gloo xDS server
* [glooctl proxy stats](../glooctl_proxy_stats)	 - stats for one of the proxy instances
* [glooctl proxy url](../glooctl_proxy_url)	 - print the http endpoint for a proxy

