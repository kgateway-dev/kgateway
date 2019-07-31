---
title: "glooctl create secret azure"
weight: 5
---
## glooctl create secret azure

Create an Azure secret with the given name

### Synopsis

Create an Azure secret with the given name

```
glooctl create secret azure [flags]
```

### Options

```
      --api-keys strings   comma-separated list of azure api key=value entries
  -h, --help               help for azure
```

### Options inherited from parent commands

```
      --dry-run             print kubernetes-formatted yaml rather than creating or updating a resource
  -i, --interactive         use interactive mode
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType   output format: (yaml, json, table, kube-yaml) (default yaml)
```

### SEE ALSO

* [glooctl create secret](../glooctl_create_secret)	 - Create a secret

