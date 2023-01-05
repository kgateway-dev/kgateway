---
title: "glooctl create upstream"
weight: 5
---
## glooctl create upstream

Create an Upstream

### Synopsis

Upstreams represent destination for routing HTTP requests. Upstreams can be compared to 
[clusters](https://www.envoyproxy.io/docs/envoy/v1.11.2/intro/arch_overview/upstream/cluster_manager#cluster-manager) in Envoy terminology. 
Each upstream in Gloo has a type. Supported types include `static`, `kubernetes`, `aws`, `consul`, and more. 
Each upstream type is handled by a corresponding Gloo plugin. 


```
glooctl create upstream [flags]
```

### Options

```
  -h, --help   help for upstream
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

* [glooctl create](../glooctl_create)	 - Create a Gloo resource
* [glooctl create upstream aws](../glooctl_create_upstream_aws)	 - Create an Aws Upstream
* [glooctl create upstream azure](../glooctl_create_upstream_azure)	 - Create an Azure Upstream
* [glooctl create upstream consul](../glooctl_create_upstream_consul)	 - Create a Consul Upstream
* [glooctl create upstream ec2](../glooctl_create_upstream_ec2)	 - Create an EC2 Upstream
* [glooctl create upstream kube](../glooctl_create_upstream_kube)	 - Create a Kubernetes Upstream
* [glooctl create upstream static](../glooctl_create_upstream_static)	 - Create a Static Upstream

