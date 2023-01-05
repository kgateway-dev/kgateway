---
title: "glooctl create upstream kube"
weight: 5
---
## glooctl create upstream kube

Create a Kubernetes Upstream

### Synopsis

Kubernetes Upstreams represent a collection of endpoints for Services registered with Kubernetes. Typically, Gloo will automatically discover these upstreams, meaning you don't have to create them. However, if upstream discovery in Gloo is disabled, or RBAC permissions have not been granted to Gloo to read from the registry, Kubernetes services can be added to Gloo manually via the CLI.

```
glooctl create upstream kube [flags]
```

### Options

```
  -h, --help                            help for kube
      --kube-service string             name of the kubernetes service
      --kube-service-labels strings     comma-separated list of labels (key=value) to use for customized selection of pods for this upstream. can be used to select subsets of pods for a service e.g. for blue-green deployment
      --kube-service-namespace string   namespace where the kubernetes service lives (default "default")
      --kube-service-port uint32        the port exposed by the kubernetes service. for services with multiple ports, create an upstream for each port. (default 80)
      --service-spec-type string        if set, Gloo supports additional routing features to upstreams with a service spec. The service spec defines a set of features 
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

