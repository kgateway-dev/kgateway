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
      --all                Deletes all gloo resour	ces, including the namespace, crds, and cluster role
      --delete-crds        Delete all gloo crds (all custom gloo objects will be deleted)
      --delete-namespace   Delete the namespace (all objects written to this namespace will be deleted)
  -h, --help               help for uninstall
  -n, --namespace string   namespace in which Gloo is installed (default "gloo-system")
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
```

### SEE ALSO

* [glooctl](../glooctl)	 - CLI for Gloo

