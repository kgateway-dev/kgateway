---
title: "glooctl create authconfig"
weight: 5
---
## glooctl create authconfig

Create an Auth Config

### Synopsis

When using Gloo Enterprise, the Gloo extauth server can be configured with numerous types of auth schemes. This configuration lives on top-level AuthConfig resources, which can be referenced from your virtual services. Virtual service auth settings can be overridden at the route or weighted destination level. Auth schemes can be chained together and executed in order, e.g. oauth, apikey auth, and more.

```
glooctl create authconfig [flags]
```

### Options

```
      --apikey-label-selector strings               apikey label selector to identify valid apikeys for this virtual service; a comma-separated list of labels (key=value)
      --apikey-secret-name string                   name to search for in provided namespace for an individual apikey secret
      --apikey-secret-namespace string              namespace to search for an individual apikey secret
      --auth-endpoint-query-params stringToString   additional static query parameters to include in authorization request to identity provider (default [])
      --enable-apikey-auth                          enable apikey auth features for this virtual service
      --enable-oidc-auth                            enable oidc auth features for this virtual service
      --enable-opa-auth                             enable opa auth features for this virtual service
  -h, --help                                        help for authconfig
      --name string                                 name of the resource to read or write
  -n, --namespace string                            namespace for reading or writing resources (default "gloo-system")
      --oidc-auth-app-url string                    the public url of your app
      --oidc-auth-callback-path string              the callback path. relative to the app url. (default "/oidc-gloo-callback")
      --oidc-auth-client-id string                  client id as registered with id provider
      --oidc-auth-client-secret-name string         name of the 'client secret' secret
      --oidc-auth-client-secret-namespace string    namespace of the 'client secret' secret
      --oidc-auth-issuer-url string                 the url of the issuer
      --oidc-scope strings                          scopes to request in addition to 'openid'. optional.
      --opa-module-ref strings                      namespace.name references to a config map containing OPA modules
      --opa-query string                            The OPA query to evaluate on a request
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
      --dry-run                    print kubernetes-formatted yaml rather than creating or updating a resource
  -i, --interactive                use interactive mode
      --kube-context string        kube context to use when interacting with kubernetes
      --kubeconfig string          kubeconfig to use, if not standard one
  -o, --output OutputType          output format: (yaml, json, table, kube-yaml, wide) (default table)
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl create](../glooctl_create)	 - Create a Gloo resource

