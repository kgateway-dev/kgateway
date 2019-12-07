---
title: "glooctl install knative"
weight: 5
---
## glooctl install knative

install Knative with Gloo on Kubernetes

### Synopsis

requires kubectl to be installed

```
glooctl install knative [flags]
```

### Options

```
  -h, --help                            help for knative
  -e, --install-eventing                Bundle Knative-Eventing with your Gloo installation. Requires install-knative to be true
      --install-eventing-version true   Version of Knative Eventing to install, when --install-eventing is set to true (default "0.10.0")
  -k, --install-knative                 Bundle Knative-Serving with your Gloo installation (default true)
      --install-knative-version true    Version of Knative Serving to install, when --install-knative is set to true. This version will also be used to install Knative Monitoring, --install-monitoring is set (default "0.10.0")
  -m, --install-monitoring              Bundle Knative-Monitoring with your Gloo installation. Requires install-knative to be true
  -g, --skip-installing-gloo            Skip installing Gloo. Only Knative components will be installed
```

### Options inherited from parent commands

```
  -c, --config string         set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
  -d, --dry-run               Dump the raw installation yaml instead of applying it to kubernetes
  -f, --file string           Install Gloo from this Helm chart archive file rather than from a release
  -i, --interactive           use interactive mode
      --kubeconfig string     kubeconfig to use, if not standard one
  -n, --namespace string      namespace to install gloo into (default "gloo-system")
      --release-name string   helm release name (default "gloo")
  -u, --upgrade               Upgrade an existing v1 gateway installation to use v2 CRDs. Set this when upgrading from v0.17.x or earlier versions of gloo
      --values strings        List of files with value overrides for the Gloo Helm chart, (e.g. --values file1,file2 or --values file1 --values file2)
  -v, --verbose               If true, output from kubectl commands will print to stdout/stderr
      --with-admin-console    install gloo and a read-only version of its admin console
```

### SEE ALSO

* [glooctl install](../glooctl_install)	 - install gloo on different platforms

