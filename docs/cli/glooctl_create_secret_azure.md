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
  -i, --interactive        use interactive mode
  -k, --kubeyaml           print kubernetes-formatted yaml rather than creating or updating a resource
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
```

### SEE ALSO

* [glooctl create secret](../glooctl_create_secret)	 - Create a secret

