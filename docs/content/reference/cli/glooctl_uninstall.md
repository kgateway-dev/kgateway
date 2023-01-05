---
title: "glooctl uninstall"
weight: 5
---
## glooctl uninstall

uninstall gloo

```
glooctl uninstall [flags]
```

### Options

```
      --all                   Deletes all gloo resources, including the namespace, crds, and cluster role
      --delete-crds           Delete all gloo crds (all custom gloo objects will be deleted)
      --delete-namespace      Delete the namespace (all objects written to this namespace will be deleted)
  -h, --help                  help for uninstall
  -n, --namespace string      namespace in which Gloo is installed (default "gloo-system")
      --release-name string   helm release name (default "gloo")
  -v, --verbose               If true, output from kubectl commands will print to stdout/stderr
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

* [glooctl](../glooctl)	 - CLI for Gloo

