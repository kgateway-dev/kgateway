---
title: "glooctl edit virtualservice"
weight: 5
---
## glooctl edit virtualservice

edit a virtualservice in a namespace

### Synopsis

usage: glooctl edit virtualservice [NAME] [--namespace=namespace] [-o FORMAT]

```
glooctl edit virtualservice [flags]
```

### Options

```
  -h, --help                          help for virtualservice
      --ssl-remove                    Remove SSL configuration from this virtual service
      --ssl-secret-name string        name of the ssl secret for this virtual service
      --ssl-secret-namespace string   namespace of the ssl secret for this virtual service
      --ssl-sni-domains stringArray   SNI domains for this virtual service
```

### Options inherited from parent commands

```
  -i, --interactive               use interactive mode
      --name string               name of the resource to read or write
  -n, --namespace string          namespace for reading or writing resources (default "gloo-system")
  -o, --output string             output format: (yaml, json, table)
      --resource-version string   the resource version of the resource we are editing. if not empty, resource will only be changed if the resource version matches
```

### SEE ALSO

* [glooctl edit](../glooctl_edit)	 - Edit a Gloo resource

