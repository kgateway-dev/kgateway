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
  -h, --help          help for proxy
  -p, --name string   the name of the proxy service/deployment to use (default "gateway-proxy")
      --port string   the name of the service port to connect to (default "http")
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
```

### SEE ALSO

* [glooctl](glooctl)	 - CLI for Gloo
* [glooctl proxy dump](glooctl_proxy_dump)	 - dump Envoy config from one of the proxy instances
* [glooctl proxy logs](glooctl_proxy_logs)	 - dump Envoy logs from one of the proxy instancesNote: this will enable verbose logging on Envoy
* [glooctl proxy stats](glooctl_proxy_stats)	 - stats for one of the proxy instances
* [glooctl proxy url](glooctl_proxy_url)	 - print the http endpoint for a proxy

