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
  -d, --dry-run                         Dump the raw installation yaml instead of applying it to kubernetes
  -f, --file string                     Install Gloo from this Helm chart archive file rather than from a release
  -h, --help                            help for knative
  -b, --install-build                   Bundle Knative-Build with your Gloo installation. Requires install-knative to be true
      --install-build-version true      Version of Knative Build to install, when --install-build is set to true (default "0.7.0")
  -e, --install-eventing                Bundle Knative-Eventing with your Gloo installation. Requires install-knative to be true
      --install-eventing-version true   Version of Knative Eventing to install, when --install-eventing is set to true (default "0.7.0")
  -k, --install-knative                 Bundle Knative-Serving with your Gloo installation (default true)
      --install-knative-version true    Version of Knative Serving to install, when --install-knative is set to true. This version will also be used to install Knative Monitoring, --install-monitoring is set (default "0.8.0")
  -m, --install-monitoring              Bundle Knative-Monitoring with your Gloo installation. Requires install-knative to be true
  -n, --namespace string                namespace to install gloo into (default "gloo-system")
  -g, --skip-installing-gloo            Skip installing Gloo. Only Knative components will be installed
  -u, --upgrade                         Upgrade an existing v1 gateway installation to use v2 CRDs. Set this when upgrading from v0.17.x or earlier versions of gloo
      --values string                   Values for the Gloo Helm chart
      --with-admin-console              install gloo and a read-only version of its admin console
```

### Options inherited from parent commands

```
  -i, --interactive         use interactive mode
      --kubeconfig string   kubeconfig to use, if not standard one
  -v, --verbose             If true, output from kubectl commands will print to stdout/stderr
```

### SEE ALSO

* [glooctl install](../glooctl_install)	 - install gloo on different platforms

