---
title: "glooctl create upstream azure"
weight: 5
---
## glooctl create upstream azure

Create an Azure Upstream

### Synopsis

Azure Upstreams represent a set of Azure Functions for a Function App that can be routed to with Gloo. Azure Upstreams require a valid set of Azure Credentials to be provided. These should be uploaded to Gloo using `glooctl create secret azure`

```
glooctl create upstream azure [flags]
```

### Options

```
      --azure-app-name string                                       name of the Azure Functions app to associate with this upstream
      --azure-secret-name glooctl create secret azure --help        name of a secret containing Azure credentials created with glooctl. See glooctl create secret azure --help for help creating secrets
      --azure-secret-namespace glooctl create secret azure --help   namespace where the Azure secret lives. See glooctl create secret azure --help for help creating secrets (default "gloo-system")
  -h, --help                                                        help for azure
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
      --name string                name of the resource to read or write
  -n, --namespace string           namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType          output format: (yaml, json, table, kube-yaml, wide) (default table)
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl create upstream](../glooctl_create_upstream)	 - Create an Upstream

