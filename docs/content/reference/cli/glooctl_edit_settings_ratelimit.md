---
title: "glooctl edit settings ratelimit"
weight: 5
---
## glooctl edit settings ratelimit

Configure rate limit settings (Enterprise)

### Synopsis

Let gloo know the location of the rate limit server. This is a Gloo Enterprise feature.

```
glooctl edit settings ratelimit [flags]
```

### Options

```
      --deny-on-failure                     On a failure to contact rate limit server, or on a timeout - deny the request (default is to allow) (default nil)
  -h, --help                                help for ratelimit
      --ratelimit-server-name string        name of the ext rate limit upstream
      --ratelimit-server-namespace string   namespace of the ext rate limit upstream
      --request-timeout duration            The timeout of the request to the rate limit server. set to 0 to use the default.
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
  -o, --output OutputType          output format: (yaml, json, table, kube-yaml, wide) (default table)
      --resource-version string    the resource version of the resource we are editing. if not empty, resource will only be changed if the resource version matches
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl edit settings](../glooctl_edit_settings)	 - root command for settings
* [glooctl edit settings ratelimit server-config](../glooctl_edit_settings_ratelimit_server-config)	 - Add rate-limit descriptor settings (Enterprise)

