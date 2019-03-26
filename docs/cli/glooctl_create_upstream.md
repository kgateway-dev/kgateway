---
title: "glooctl create upstream"
weight: 5
---
## glooctl create upstream

Create an Upstream Interactively

### Synopsis

Upstreams represent destination for routing HTTP requests. Upstreams can be compared to 
[clusters](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/cluster_manager) in Envoy terminology. 
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
  -i, --interactive        use interactive mode
  -k, --kubeyaml           print kubernetes-formatted yaml rather than creating or updating a resource
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
```

### SEE ALSO

* [glooctl create](../glooctl_create)	 - Create a Gloo resource
* [glooctl create upstream aws](../glooctl_create_upstream_aws)	 - Create an Aws Upstream
* [glooctl create upstream azure](../glooctl_create_upstream_azure)	 - Create an Azure Upstream
* [glooctl create upstream consul](../glooctl_create_upstream_consul)	 - Create a Consul Upstream
* [glooctl create upstream kube](../glooctl_create_upstream_kube)	 - Create a Kubernetes Upstream
* [glooctl create upstream static](../glooctl_create_upstream_static)	 - Create a Static Upstream

