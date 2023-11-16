---
title: "glooctl uninstall"
weight: 5
---
## glooctl uninstall

uninstall gloo

```
glooctl uninstall [flags]
```

### Options

```
      --dry-run string     print the generated kubernetes manifest to stdout
  -g, --gateway            install the default gloo gateway proxy, (default false)
  -h, --help               help for uninstall
  -n, --namespace string   namespace in which Gloo is installed (default "gloo-system")
      --set strings        directly set values for the gloo gateway helm chart (can be repeated or comma separated list of values)
      --values strings     path to a helm values file (can be repeated or comma separated list of values)
  -v, --verbose            If true, output from kubectl commands will print to stdout/stderr
```

### Options inherited from parent commands

```
  -c, --config string         set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --kube-context string   kube context to use when interacting with kubernetes
      --kubeconfig string     kubeconfig to use, if not standard one
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo

