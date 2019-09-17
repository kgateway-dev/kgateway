---
title: "glooctl create virtualservice"
weight: 5
---
## glooctl create virtualservice

Create a Virtual Service

### Synopsis

A virtual service describes the set of routes to match for a set of domains. 
Virtual services are containers for routes assigned to a domain or set of domains. 
Virtual services must not have overlapping domains, as the virtual service to match a request is selected by the Host header (in HTTP1) or :authority header (in HTTP2). When using Gloo Enterprise, virtual services can be configured with rate limiting, oauth, and apikey auth.

```
glooctl create virtualservice [flags]
```

### Options

```
      --apikey-label-selector strings              apikey label selector to identify valid apikeys for this virtual service; a comma-separated list of labels (key=value)
      --apikey-secret-name string                  name to search for in provided namespace for an individual apikey secret
      --apikey-secret-namespace string             namespace to search for an individual apikey secret
      --consul-address string                      address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-datacenter string                   Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string                     key prefix for for Consul key-value storage. (default "gloo")
      --consul-scheme string                       URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string                        Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
      --display-name string                        descriptive name of virtual service (defaults to resource name)
      --domains strings                            comma separated list of domains
      --enable-apikey-auth                         enable apikey auth features for this virtual service
      --enable-oidc-auth                           enable oidc auth features for this virtual service
      --enable-opa-auth                            enable opa auth features for this virtual service
      --enable-rate-limiting                       enable rate limiting features for this virtual service
  -h, --help                                       help for virtualservice
      --oidc-auth-app-url string                   the public url of your app
      --oidc-auth-callback-path string             the callback path. relative to the app url. (default "/oidc-gloo-callback")
      --oidc-auth-client-id string                 client id as registered with id provider
      --oidc-auth-client-secret-name string        name of the 'client secret' secret
      --oidc-auth-client-secret-namespace string   namespace of the 'client secret' secret
      --oidc-auth-issuer-url string                the url of the issuer
      --oidc-scope strings                         scopes to request in addition to 'openid'. optional.
      --opa-module-ref strings                     namespace.name references to a config map containing OPA modules
      --opa-query string                           The OPA query to evaluate on a request
      --rate-limit-requests uint32                 requests per unit of time (default 100)
      --rate-limit-time-unit string                unit of time over which to apply the rate limit (default "MINUTE")
      --use-consul                                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### Options inherited from parent commands

```
      --dry-run             print kubernetes-formatted yaml rather than creating or updating a resource
  -i, --interactive         use interactive mode
      --kubeconfig string   kubeconfig to use, if not standard one
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType   output format: (yaml, json, table, kube-yaml, wide) (default table)
```

### SEE ALSO

* [glooctl create](../glooctl_create)	 - Create a Gloo resource

