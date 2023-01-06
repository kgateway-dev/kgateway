---
title: "glooctl check"
weight: 5
---
## glooctl check

Checks Gloo resources for errors (requires Gloo running on Kubernetes)

### Synopsis

usage: glooctl check [-o FORMAT]

```
glooctl check [flags]
```

### Options

```
  -x, --exclude strings                   check to exclude: (deployments, pods, upstreams, upstreamgroup, auth-configs, rate-limit-configs, secrets, virtual-services, gateways, proxies, xds-metrics)
  -h, --help                              help for check
  -n, --namespace string                  namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType                 output format: (json, table) (default table)
  -p, --pod-selector string               Label selector for pod scanning (default "gloo")
      --read-only                         only do checks that dont require creating resources (i.e. port forwards)
  -r, --resource-namespaces stringArray   Namespaces in which to scan gloo custom resources. If not provided, all watched namespaces (as specified in settings) will be scanned.
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

