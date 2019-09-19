---
title: "glooctl debug logs"
weight: 5
---
## glooctl debug logs

Debug Gloo logs (requires Gloo running on Kubernetes)

### Synopsis

Debug Gloo logs (requires Gloo running on Kubernetes)

```
glooctl debug logs [flags]
```

### Options

```
      --errors-only        filter for error logs only
  -f, --file string        file to be read or written to
  -h, --help               help for logs
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
      --zip                save logs to a tar file (specify location with -f)
```

### Options inherited from parent commands

```
  -i, --interactive         use interactive mode
      --kubeconfig string   kubeconfig to use, if not standard one
```

### SEE ALSO

* [glooctl debug](../glooctl_debug)	 - Debug a Gloo resource (requires Gloo running on Kubernetes)

