---
title: "glooctl"
weight: 5
---
## glooctl

CLI for Gloo Edge

### Synopsis

glooctl is the unified CLI for Gloo Edge.
	Find more information at https://solo.io

### Options

```
  -c, --config string              set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --consul-address string      address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-datacenter string   Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string     key prefix for for Consul key-value storage. (default "gloo")
      --consul-scheme string       URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string        Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
  -h, --help                       help for glooctl
  -i, --interactive                use interactive mode
      --kubeconfig string          kubeconfig to use, if not standard one
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl add](../glooctl_add)	 - Adds configuration to a top-level Gloo Edge resource
* [glooctl check](../glooctl_check)	 - Checks Gloo Edge resources for errors (requires Gloo Edge running on Kubernetes)
* [glooctl cluster](../glooctl_cluster)	 - Cluster commands
* [glooctl completion](../glooctl_completion)	 - generate auto completion for your shell
* [glooctl create](../glooctl_create)	 - Create a Gloo Edge resource
* [glooctl dashboard](../glooctl_dashboard)	 - Open Gloo Edge dashboard
* [glooctl debug](../glooctl_debug)	 - Debug a Gloo Edge resource (requires Gloo Edge running on Kubernetes)
* [glooctl delete](../glooctl_delete)	 - Delete a Gloo Edge resource
* [glooctl demo](../glooctl_demo)	 - Demos (requires 4 tools to be installed and accessible via the PATH: glooctl, kubectl, docker, and kind.)
* [glooctl edit](../glooctl_edit)	 - Edit a Gloo Edge resource
* [glooctl get](../glooctl_get)	 - Display one or a list of Gloo Edge resources
* [glooctl install](../glooctl_install)	 - install gloo on different platforms
* [glooctl istio](../glooctl_istio)	 - Commands for interacting with Istio in Gloo Edge
* [glooctl plugin](../glooctl_plugin)	 - Commands for interacting with glooctl plugins
* [glooctl proxy](../glooctl_proxy)	 - interact with proxy instances managed by Gloo Edge
* [glooctl remove](../glooctl_remove)	 - remove configuration items from a top-level Gloo Edge resource
* [glooctl route](../glooctl_route)	 - subcommands for interacting with routes within virtual services
* [glooctl uninstall](../glooctl_uninstall)	 - uninstall gloo
* [glooctl upgrade](../glooctl_upgrade)	 - upgrade glooctl binary
* [glooctl version](../glooctl_version)	 - Print current version

