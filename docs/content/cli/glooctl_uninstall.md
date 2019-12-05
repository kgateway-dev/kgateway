---
title: "glooctl uninstall"
weight: 5
---
## glooctl uninstall

uninstall gloo

### Synopsis

uninstall gloo

```
glooctl uninstall [flags]
```

### Options

```
      --all                Deletes all gloo resources, including the namespace, crds, and cluster role
      --delete-crds        Delete all gloo crds (all custom gloo objects will be deleted)
      --delete-namespace   Delete the namespace (all objects written to this namespace will be deleted)
      --force              Uninstalls Gloo even if the installation ID cannot be determined from the gloo pod labels (using this may delete cluster-scoped resources belonging to other Gloo installations)
  -h, --help               help for uninstall
  -n, --namespace string   namespace in which Gloo is installed (default "gloo-system")
  -v, --verbose            If true, output from kubectl commands will print to stdout/stderr
```

### Options inherited from parent commands

```
  -c, --config string       set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
  -i, --interactive         use interactive mode
      --kubeconfig string   kubeconfig to use, if not standard one
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo

