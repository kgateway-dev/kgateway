---
title: "glooctl remove route"
weight: 5
---
## glooctl remove route

Remove a Route from a Virtual Service

### Synopsis

Routes match patterns on requests and indicate the type of action to take when a proxy receives a matching request. Requests can be broken down into their Match and Action components. The order of routes within a Virtual Service matters. The first route in the virtual service that matches a given request will be selected for routing. 

If no virtual service is specified for this command, glooctl add route will attempt to add it to a default virtualservice with domain '*'. if one does not exist, it will be created for you.

Usage: `glooctl rm route [--name virtual-service-name] [--namespace namespace] [--index x]`

```
glooctl remove route [flags]
```

### Options

```
  -h, --help                help for route
  -x, --index uint32        remove the route with this index in the virtual service route list
  -o, --output OutputType   output format: (yaml, json, table, kube-yaml, wide) (default table)
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
      --name string                name of the resource to read or write
  -n, --namespace string           namespace for reading or writing resources (default "gloo-system")
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl remove](../glooctl_remove)	 - remove configuration items from a top-level Gloo resource

