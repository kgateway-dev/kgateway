---
title: "glooctl upgrade"
weight: 5
---
## glooctl upgrade

upgrade glooctl binary

```
glooctl upgrade [flags]
```

### Options

```
  -h, --help             help for upgrade
      --path string      Desired path for your upgraded glooctl binary. Defaults to the location of your currently executing binary.
      --release string   Which glooctl release to download. Specify a release tag corresponding to the desired version of glooctl,"experimental" to use the version currently under development, or a major+minor release like v1.10.x to get the most recent patch version. (default "latest")
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

