---
title: "glooctl check"
weight: 5
---
## glooctl check

Checks Gloo resources for errors (requires Gloo running on Kubernetes)

### Synopsis

usage: glooctl check [-o FORMAT]

```
glooctl check [flags]
```

### Options

```
  -x, --exclude strings                   check to exclude: (deployments, pods, upstreams, upstreamgroup, auth-configs, rate-limit-configs, secrets, virtual-services, gateways, proxies, xds-metrics)
  -h, --help                              help for check
  -n, --namespace string                  namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType                 output format: (json, table) (default table)
      --read-only                         only do checks that dont require creating resources (i.e. port forwards)
  -r, --resource-namespaces stringArray   Namespaces in which to scan gloo custom resources. If not provided, all watched namespaces (as specified in settings) will be scanned.
```

### Options inherited from parent commands

```
  -c, --config string         set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --kube-context string   kube context to use when interacting with kubernetes
      --kubeconfig string     kubeconfig to use, if not standard one
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo

