---
title: "glooctl edit route externalauth"
weight: 5
---
## glooctl edit route externalauth

Configure disable external auth on a route (Enterprise)

### Synopsis

Allows disabling external auth on specific routes. External auth is a gloo enterprise feature.

```
glooctl edit route externalauth [flags]
```

### Options

```
  -d, --disable            set to true to disable authentication on this route
  -h, --help               help for externalauth
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
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
  -x, --index uint32               edit the route with this index in the virtual service route list
  -i, --interactive                use interactive mode
      --kube-context string        kube context to use when interacting with kubernetes
      --kubeconfig string          kubeconfig to use, if not standard one
  -o, --output OutputType          output format: (yaml, json, table, kube-yaml, wide) (default table)
      --resource-version string    the resource version of the resource we are editing. if not empty, resource will only be changed if the resource version matches
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl edit route](../glooctl_edit_route)	 - 

