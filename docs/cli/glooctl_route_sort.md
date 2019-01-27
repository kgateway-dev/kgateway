## glooctl route sort

sort routes on an existing virtual service

### Synopsis

The order of routes matters. A route is selected for a request based on the first matching route matcher in the virtual serivce's list. sort automatically sorts the routes in the virtual service

Usage: `glooctl route sort [--name virtual-service-name] [--namespace virtual-service-namespace]`

```
glooctl route sort [flags]
```

### Options

```
  -h, --help            help for sort
  -o, --output string   output format: (yaml, json, table)
```

### Options inherited from parent commands

```
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
```

### SEE ALSO

* [glooctl route](glooctl_route.md)	 - subcommands for interacting with routes within virtual services

