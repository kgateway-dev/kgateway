---
title: "glooctl"
weight: 5
---
## glooctl

CLI for Gloo

### Synopsis

glooctl is the unified CLI for Gloo.
	Find more information at https://solo.io

### Options

```
  -c, --config string              set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --consul-address string      address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-allow-stale-reads   Allows reading using Consul's stale consistency mode.
      --consul-datacenter string   Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string     key prefix for for Consul key-value storage. (default "gloo")
      --consul-scheme string       URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string        Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
  -h, --help                       help for glooctl
  -i, --interactive                use interactive mode
      --kube-context string        kube context to use when interacting with kubernetes
      --kubeconfig string          kubeconfig to use, if not standard one
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl check](../glooctl_check)	 - Checks Gloo resources for errors (requires Gloo running on Kubernetes)
* [glooctl completion](../glooctl_completion)	 - generate auto completion for your shell
* [glooctl demo](../glooctl_demo)	 - Demos (requires 4 tools to be installed and accessible via the PATH: glooctl, kubectl, docker, and kind.)
* [glooctl get](../glooctl_get)	 - Display one or a list of Gloo resources
* [glooctl install](../glooctl_install)	 - install gloo on different platforms
* [glooctl uninstall](../glooctl_uninstall)	 - uninstall gloo
* [glooctl upgrade](../glooctl_upgrade)	 - upgrade glooctl binary
* [glooctl version](../glooctl_version)	 - Print current version

