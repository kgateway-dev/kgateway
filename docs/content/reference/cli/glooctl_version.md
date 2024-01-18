---
title: "glooctl version"
weight: 5
---
## glooctl version

Print current version

### Synopsis

Get the version of Glooctl and Gloo

```
glooctl version [flags]
```

### Options

```
  -h, --help                help for version
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType   output format: (yaml, json, table, kube-yaml, wide) (default json)
```

### Options inherited from parent commands

```
  -c, --config string         set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --kube-context string   kube context to use when interacting with kubernetes
      --kubeconfig string     kubeconfig to use, if not standard one
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo

