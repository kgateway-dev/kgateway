---
title: "glooctl create virtualservice"
weight: 5
---
## glooctl create virtualservice

Create a Virtual Service

### Synopsis

A virtual service describes the set of routes to match for a set of domains. 
Virtual services are containers for routes assigned to a domain or set of domains. 
Virtual services must not have overlapping domains, as the virtual service to match a request is selected by the Host header (in HTTP1) or :authority header (in HTTP2). The routes within a virtual service 

```
glooctl create virtualservice [flags]
```

### Options

```
      --consul-address string      address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-datacenter string   Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string     key prefix for for Consul key-value storage. (default "gloo")
      --consul-scheme string       URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string        Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
      --display-name string        descriptive name of virtual service (defaults to resource name)
      --domains strings            comma separated list of domains
  -h, --help                       help for virtualservice
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### Options inherited from parent commands

```
      --dry-run             print kubernetes-formatted yaml rather than creating or updating a resource
  -i, --interactive         use interactive mode
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType   output format: (yaml, json, table, kube-yaml, wide) (default table)
```

### SEE ALSO

* [glooctl create](../glooctl_create)	 - Create a Gloo resource

