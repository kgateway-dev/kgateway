---
title: "glooctl create"
weight: 5
---
## glooctl create

Create a Gloo resource

### Synopsis

Gloo resources be created from files (including stdin)

```
glooctl create [flags]
```

### Options

```
      --dry-run             print kubernetes-formatted yaml rather than creating or updating a resource
  -f, --file string         file to be read or written to
  -h, --help                help for create
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType   output format: (yaml, json, table, kube-yaml, wide) (default table)
```

### Options inherited from parent commands

```
  -i, --interactive         use interactive mode
      --kubeconfig string   kubeconfig to use, if not standard one
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo
* [glooctl create secret](../glooctl_create_secret)	 - Create a secret
* [glooctl create upstream](../glooctl_create_upstream)	 - Create an Upstream
* [glooctl create upstreamgroup](../glooctl_create_upstreamgroup)	 - Create an Upstream Group
* [glooctl create virtualservice](../glooctl_create_virtualservice)	 - Create a Virtual Service

