---
title: "glooctl add"
weight: 5
---
## glooctl add

Adds configuration to a top-level Gloo resource.

### Synopsis

Adds configuration to a top-level Gloo resource.

```
glooctl add [flags]
```

### Options

```
      --dry-run             print kubernetes-formatted yaml rather than creating or updating a resource
  -h, --help                help for add
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType   output format: (yaml, json, table, kube-yaml) (default table)
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo
* [glooctl add route](../glooctl_add_route)	 - Add a Route to a Virtual Service

