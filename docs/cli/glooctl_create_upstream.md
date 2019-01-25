## glooctl create upstream

Create an Upstream Interactively

### Synopsis

Upstreams represent destination for routing HTTP requests. Upstreams can be compared to 
[clusters](https://www.envoyproxy.io/docs/envoy/latest/api-v1/cluster_manager/cluster.html?highlight=cluster) in Envoy terminology. 
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
  -i, --interactive     use interactive mode
  -o, --output string   output format: (yaml, json, table)
```

### SEE ALSO

* [glooctl create](glooctl_create.md)	 - Create a Gloo resource
* [glooctl create upstream aws](glooctl_create_upstream_aws.md)	 - Create an Aws Upstream
* [glooctl create upstream azure](glooctl_create_upstream_azure.md)	 - Create an Azure Upstream
* [glooctl create upstream consul](glooctl_create_upstream_consul.md)	 - Create a Consul Upstream
* [glooctl create upstream kube](glooctl_create_upstream_kube.md)	 - Create a Kubernetes Upstream
* [glooctl create upstream static](glooctl_create_upstream_static.md)	 - Create a Static Upstream

