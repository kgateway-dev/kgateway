---
title: "glooctl cluster register"
weight: 5
---
## glooctl cluster register

Register a cluster to the Gloo Federation control plane

### Synopsis

Register a cluster to the Gloo Federation control plane. Registered clusters can be targeted for discovery and configuration.

```
glooctl cluster register [flags]
```

### Options

```
      --cluster-name string           name of the cluster to register
      --federation-namespace string   namespace of the Gloo Federation control plane (default "gloo-system")
  -h, --help                          help for register
      --remote-context string         name of the kubeconfig context to use for registration
      --remote-kubeconfig string      path to the kubeconfig from which the registered cluster will be accessed
      --remote-namespace string       namespace in the target cluster where registration artifacts should be written (default "gloo-system")
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

* [glooctl cluster](../glooctl_cluster)	 - Cluster commands

