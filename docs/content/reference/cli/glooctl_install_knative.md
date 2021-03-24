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
      --create-namespace                Create the namespace to install gloo into (default true)
  -d, --dry-run                         Dump the raw installation yaml instead of applying it to kubernetes
  -f, --file string                     Install Gloo from this Helm chart archive file rather than from a release
  -h, --help                            help for knative
  -e, --install-eventing                Bundle Knative-Eventing with your Gloo installation. Requires install-knative to be true
      --install-eventing-version true   Version of Knative Eventing to install, when --install-eventing is set to true (default "0.10.0")
  -k, --install-knative                 Bundle Knative-Serving with your Gloo installation (default true)
      --install-knative-version true    Version of Knative Serving to install, when --install-knative is set to true. This version will also be used to install Knative Monitoring, --install-monitoring is set (default "0.10.0")
  -m, --install-monitoring              Bundle Knative-Monitoring with your Gloo installation. Requires install-knative to be true
  -n, --namespace string                namespace to install gloo into (default "gloo-system")
      --release-name string             helm release name (default "gloo")
  -g, --skip-installing-gloo            Skip installing Gloo Edge. Only Knative components will be installed
      --values strings                  List of files with value overrides for the Gloo Helm chart, (e.g. --values file1,file2 or --values file1 --values file2)
      --version string                  version to install (e.g. 1.4.0, defaults to latest)
```

### Options inherited from parent commands

```
  -c, --config string              set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --consul-address string      address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-datacenter string   Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string     key prefix for for Consul key-value storage. (default "gloo")
      --consul-scheme string       URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string        Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
  -i, --interactive                use interactive mode
      --kubeconfig string          kubeconfig to use, if not standard one
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
  -v, --verbose                    If true, output from kubectl commands will print to stdout/stderr
```

### SEE ALSO

* [glooctl install](../glooctl_install)	 - install gloo on different platforms

