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
      --dry-run            print kubernetes-formatted yaml rather than creating or updating a resource
  -f, --file string        file to be read or written to
  -h, --help               help for create
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
      --yaml               print basic (non-kubernetes) yaml rather than creating or updating a resource
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo
* [glooctl create secret](../glooctl_create_secret)	 - Create a secret
* [glooctl create upstream](../glooctl_create_upstream)	 - Create an Upstream
* [glooctl create virtualservice](../glooctl_create_virtualservice)	 - Create a Virtual Service

