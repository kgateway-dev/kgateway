---
title: "glooctl install knative"
weight: 5
---
## glooctl install knative

install Knative with Gloo on kubernetes

### Synopsis

requires kubectl to be installed

```
glooctl install knative [flags]
```

### Options

```
  -d, --dry-run            Dump the raw installation yaml instead of applying it to kubernetes
  -f, --file string        Install Gloo from this Helm chart archive file rather than from a release
  -h, --help               help for knative
  -n, --namespace string   namespace to install gloo into (default "gloo-system")
      --release string     install using this release version. defaults to the latest github release
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
```

### SEE ALSO

* [glooctl install](../glooctl_install)	 - install gloo on different platforms

