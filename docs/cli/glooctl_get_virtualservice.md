---
title: "glooctl get virtualservice"
weight: 5
---
## glooctl get virtualservice

read a virtualservice or list virtualservices in a namespace

### Synopsis

usage: glooctl get virtualservice [NAME] [--namespace=namespace] [-o FORMAT]

```
glooctl get virtualservice [flags]
```

### Options

```
  -h, --help   help for virtualservice
```

### Options inherited from parent commands

```
  -i, --interactive         use interactive mode
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType   output format: (yaml, json, table, kube-yaml, wide) (default table)
```

### SEE ALSO

* [glooctl get](../glooctl_get)	 - Display one or a list of Gloo resources
* [glooctl get virtualservice route](../glooctl_get_virtualservice_route)	 - get a list of routes for a given virtual service

