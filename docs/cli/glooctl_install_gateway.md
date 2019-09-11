---
title: "glooctl install gateway"
weight: 5
---
## glooctl install gateway

install the Gloo Gateway on kubernetes

### Synopsis

requires kubectl to be installed

```
glooctl install gateway [flags]
```

### Options

```
  -d, --dry-run              Dump the raw installation yaml instead of applying it to kubernetes
      --enterprise           Install Enterprise Gloo
  -f, --file string          Install Gloo from this Helm chart archive file rather than from a release
  -h, --help                 help for gateway
      --license-key string   License key to activate GlooE features
  -n, --namespace string     namespace to install gloo into (default "gloo-system")
  -u, --upgrade              Upgrade an existing v1 gateway installation to use v2 CRDs. Set this when upgrading from v0.17.x or earlier versions of gloo
      --values string        Values for the Gloo Helm chart
```

### Options inherited from parent commands

```
  -i, --interactive   use interactive mode
  -v, --verbose       If true, output from kubectl commands will print to stdout/stderr
```

### SEE ALSO

* [glooctl install](../glooctl_install)	 - install gloo on different platforms

