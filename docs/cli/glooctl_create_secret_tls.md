---
title: "glooctl create secret tls"
weight: 5
---
## glooctl create secret tls

Create a secret with the given name

### Synopsis

Create a secret with the given name

```
glooctl create secret tls [flags]
```

### Options

```
      --certchain string    filename of certchain for secret
  -h, --help                help for tls
      --privatekey string   filename of privatekey for secret
      --rootca string       filename of rootca for secret
```

### Options inherited from parent commands

```
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
```

### SEE ALSO

* [glooctl create secret](../glooctl_create_secret)	 - Create a secret

