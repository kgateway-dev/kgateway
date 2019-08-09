---
title: "glooctl create secret aws"
weight: 5
---
## glooctl create secret aws

Create an AWS secret with the given name

### Synopsis

Create an AWS secret with the given name

```
glooctl create secret aws [flags]
```

### Options

```
      --access-key string   aws access key
  -h, --help                help for aws
      --secret-key string   aws secret key
```

### Options inherited from parent commands

```
      --dry-run             print kubernetes-formatted yaml rather than creating or updating a resource
  -i, --interactive         use interactive mode
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType   output format: (yaml, json, table, kube-yaml) (default table)
```

### SEE ALSO

* [glooctl create secret](../glooctl_create_secret)	 - Create a secret

