---
title: "glooctl create virtualservice"
weight: 5
---
## glooctl create virtualservice

Create a Virtual Service

### Synopsis

A virtual service describes the set of routes to match for a set of domains. 
Virtual services are containers for routes assigned to a domain or set of domains. 
Virtual services must not have overlapping domains, as the virtual service to match a request is selected by the Host header (in HTTP1) or :authority header (in HTTP2). When using Gloo Enterprise, virtual services can be configured with rate limiting, oauth, apikey auth, and more.

```
glooctl create virtualservice [flags]
```

### Options

```
      --display-name string           descriptive name of virtual service (defaults to resource name)
      --domains strings               comma separated list of domains
      --enable-rate-limiting          enable rate limiting features for this virtual service
  -h, --help                          help for virtualservice
      --name string                   name of the resource to read or write
  -n, --namespace string              namespace for reading or writing resources (default "gloo-system")
      --rate-limit-requests uint32    requests per unit of time (default 100)
      --rate-limit-time-unit string   unit of time over which to apply the rate limit (default "MINUTE")
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
      --dry-run                    print kubernetes-formatted yaml rather than creating or updating a resource
  -i, --interactive                use interactive mode
      --kube-context string        kube context to use when interacting with kubernetes
      --kubeconfig string          kubeconfig to use, if not standard one
  -o, --output OutputType          output format: (yaml, json, table, kube-yaml, wide) (default table)
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl create](../glooctl_create)	 - Create a Gloo resource

