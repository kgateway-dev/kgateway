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
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
      --secret-key string   aws secret key
```

### Options inherited from parent commands

```
  -i, --interactive     use interactive mode
  -o, --output string   output format: (yaml, json, table)
```

### SEE ALSO

* [glooctl create secret](glooctl_create_secret)	 - Create a secret

