---
title: "glooctl debug yaml"
weight: 5
---
## glooctl debug yaml

Dump YAML representing the current Gloo Edge state (requires Gloo Edge running on Kubernetes)

### Synopsis

Dump YAML representing the current Gloo Edge state (requires Gloo Edge running on Kubernetes)

```
glooctl debug yaml [flags]
```

### Options

```
  -f, --file string        file to be read or written to
  -h, --help               help for yaml
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
```

### Options inherited from parent commands

```
  -i, --interactive         use interactive mode
      --kubeconfig string   kubeconfig to use, if not standard one
```

### SEE ALSO

* [glooctl debug](../glooctl_debug)	 - Debug a Gloo Edge resource (requires Gloo Edge running on Kubernetes)

