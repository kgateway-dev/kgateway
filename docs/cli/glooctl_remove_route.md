---
title: "glooctl remove route"
weight: 5
---
## glooctl remove route

Remove a Route from a Virtual Service

### Synopsis

Routes match patterns on requests and indicate the type of action to take when a proxy receives a matching request. Requests can be broken down into their Match and Action components. The order of routes within a Virtual Service matters. The first route in the virtual service that matches a given request will be selected for routing. 

If no virtual service is specified for this command, glooctl add route will attempt to add it to a default virtualservice with domain '*'. if one does not exist, it will be created for you.

Usage: `glooctl rm route [--name virtual-service-name] [--namespace namespace] [--index x]`

```
glooctl remove route [flags]
```

### Options

```
  -h, --help            help for route
  -x, --index uint32    remove the route with this index in the virtual service route list
  -o, --output string   output format: (yaml, json, table)
```

### Options inherited from parent commands

```
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
```

### SEE ALSO

* [glooctl remove](../glooctl_remove)	 - remove configuration items from a top-level Gloo resource

