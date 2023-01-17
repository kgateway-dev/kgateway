---
title: "glooctl demo federation"
weight: 5
---
## glooctl demo federation

Bootstrap a multicluster demo with Gloo Federation.

### Synopsis

Running the Gloo Federation demo setup locally requires 4 tools to be installed and accessible via the PATH: glooctl, kubectl, docker, and kind. This command will bootstrap 2 kind clusters, one of which will run the Gloo Federation management-plane as well as Gloo Enterprise, and the other will just run Gloo. Please note that cluster registration will only work on darwin and linux OS.

```
glooctl demo federation [flags]
```

### Options

```
  -h, --help                 help for federation
      --license-key string   License key to activate Gloo Fed features
      --version string       Version of Gloo Enterprise to install (defaults to latest)
```

### Options inherited from parent commands

```
  -c, --config string              set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --consul-address string      address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-allow-stale-reads   Allows reading using Consul's stale consistency mode.
      --consul-datacenter string   Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string     key prefix for for Consul key-value storage. (default "gloo")
      --consul-scheme string       URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string        Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
  -i, --interactive                use interactive mode
      --kube-context string        kube context to use when interacting with kubernetes
      --kubeconfig string          kubeconfig to use, if not standard one
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl demo](../glooctl_demo)	 - Demos (requires 4 tools to be installed and accessible via the PATH: glooctl, kubectl, docker, and kind.)

