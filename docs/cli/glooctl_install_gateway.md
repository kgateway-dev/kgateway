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
  -f, --file string          Install Gloo from this Helm chart archive file rather than from a release
  -h, --help                 help for gateway
  -n, --namespace string     namespace to install gloo into (default "gloo-system")
  -u, --upgrade              Upgrade an existing v1 gateway installation to use v2 CRDs. Set this when upgrading from v0.17.x or earlier versions of gloo
      --values string        Values for the Gloo Helm chart
      --with-admin-console   install gloo and a read-only version of its admin console
```

### Options inherited from parent commands

```
  -i, --interactive         use interactive mode
      --kubeconfig string   kubeconfig to use, if not standard one
  -v, --verbose             If true, output from kubectl commands will print to stdout/stderr
```

### SEE ALSO

* [glooctl install](../glooctl_install)	 - install gloo on different platforms
* [glooctl install gateway enterprise](../glooctl_install_gateway_enterprise)	 - install the Gloo Enterprise Gateway on kubernetes

