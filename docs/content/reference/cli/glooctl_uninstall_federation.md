---
title: "glooctl uninstall federation"
weight: 5
---
## glooctl uninstall federation

uninstall gloo federation

### Synopsis

uninstall gloo federation

```
glooctl uninstall federation [flags]
```

### Options

```
      --all                   Deletes all gloo fed resources, including the namespace, crds, and cluster role
      --delete-crds           Delete all gloo fed crds (all custom gloo fed objects will be deleted)
      --delete-namespace      Delete the namespace (all objects written to this namespace will be deleted)
  -h, --help                  help for federation
      --namespace string      namespace in which Gloo Fed is installed (default "gloo-fed")
      --release-name string   helm release name (default "gloo-fed")
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

* [glooctl uninstall](../glooctl_uninstall)	 - uninstall gloo

