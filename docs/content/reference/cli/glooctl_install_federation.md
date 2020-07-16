---
title: "glooctl install federation"
weight: 5
---
## glooctl install federation

install Gloo Federation on Kubernetes

### Synopsis

requires kubectl to be installed

```
glooctl install federation [flags]
```

### Options

```
      --create-namespace      Create the namespace to install gloo fed into (default true)
      --dry-run               Dump the raw installation yaml instead of applying it to kubernetes
      --file string           Install Gloo Fed from this Helm chart archive file rather than from a release
  -h, --help                  help for federation
      --license-key string    License key to activate Gloo Fed features
      --namespace string      namespace to install gloo fed into (default "gloo-fed")
      --release-name string   helm release name (default "gloo-fed")
      --values strings        List of files with value overrides for the Gloo Fed Helm chart, (e.g. --values file1,file2 or --values file1 --values file2)
      --version string        version to install (e.g. 0.0.6, defaults to latest)
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

