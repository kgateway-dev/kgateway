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
      --name string        the name of the proxy service/deployment to use (default "gateway-proxy")
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
      --port string        the name of the service port to connect to (default "http")
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
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo
* [glooctl proxy address](../glooctl_proxy_address)	 - print the socket address for a proxy
* [glooctl proxy dump](../glooctl_proxy_dump)	 - dump Envoy config from one of the proxy instances
* [glooctl proxy logs](../glooctl_proxy_logs)	 - dump Envoy logs from one of the proxy instancesNote: this will enable verbose logging on Envoy
* [glooctl proxy served-config](../glooctl_proxy_served-config)	 - dump Envoy config being served by the Gloo xDS server
* [glooctl proxy stats](../glooctl_proxy_stats)	 - stats for one of the proxy instances
* [glooctl proxy url](../glooctl_proxy_url)	 - print the http endpoint for a proxy

