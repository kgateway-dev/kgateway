---
title: "glooctl install ingress"
weight: 5
---
## glooctl install ingress

install the Gloo Ingress Controller on kubernetes

### Synopsis

requires kubectl to be installed

```
glooctl install ingress [flags]
```

### Options

```
  -d, --dry-run            Dump the raw installation yaml instead of applying it to kubernetes
  -f, --file string        Install Gloo from this Helm chart archive file rather than from a release
  -h, --help               help for ingress
  -n, --namespace string   namespace to install gloo into (default "gloo-system")
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
  -v, --verbose       If true, output from kubectl commands will print to stdout/stderr
```

### SEE ALSO

* [glooctl install](../glooctl_install)	 - install gloo on different platforms

