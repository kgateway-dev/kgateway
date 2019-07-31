---
title: "glooctl edit"
weight: 5
---
## glooctl edit

Edit a Gloo resource

### Synopsis

Edit a Gloo resource

### Options

```
  -h, --help                      help for edit
      --name string               name of the resource to read or write
  -n, --namespace string          namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType         output format: (yaml, json, table, kube-yaml) (default yaml)
      --resource-version string   the resource version of the resource we are editing. if not empty, resource will only be changed if the resource version matches
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo
* [glooctl edit upstream](../glooctl_edit_upstream)	 - edit an upstream in a namespace
* [glooctl edit virtualservice](../glooctl_edit_virtualservice)	 - edit a virtualservice in a namespace

