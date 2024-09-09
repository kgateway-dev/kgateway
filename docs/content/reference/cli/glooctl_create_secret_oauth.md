---
title: "glooctl create secret oauth"
weight: 5
---
## glooctl create secret oauth

Create an OAuth secret with the given name (Enterprise)

### Synopsis

Create an OAuth secret with the given name. The OAuth secret contains the client_secret as defined in [RFC 6749](https://tools.ietf.org/html/rfc6749). This is an enterprise-only feature. The format of the secret data is: `{"oauth" : [client-secret string]}`. 

```
glooctl create secret oauth [flags]
```

### Options

```
      --client-secret string   oauth client secret
  -h, --help                   help for oauth
```

### Options inherited from parent commands

```
  -c, --config string                  set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --consul-address string          address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-allow-stale-reads       Allows reading using Consul's stale consistency mode.
      --consul-datacenter string       Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string         key prefix for for Consul key-value storage. (default "gloo")
      --consul-scheme string           URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string            Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
      --dry-run                        print kubernetes-formatted yaml rather than creating or updating a resource
  -i, --interactive                    use interactive mode
      --kube-context string            kube context to use when interacting with kubernetes
      --kubeconfig string              kubeconfig to use, if not standard one
      --name string                    name of the resource to read or write
  -n, --namespace string               namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType              output format: (yaml, json, table, kube-yaml, wide) (default table)
      --use-consul                     use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
      --use-vault                      use Vault Key-Value storage as the backend for reading and writing secrets
      --vault-address string           address of the Vault server. This should be a complete URL such as "http://vault.example.com". Use with --use-vault (default "https://127.0.0.1:8200")
      --vault-ca-cert string           CACert is the path to a PEM-encoded CA cert file to use to verify the Vault server SSL certificate.Use with --use-vault
      --vault-ca-path string           CAPath is the path to a directory of PEM-encoded CA cert files to verify the Vault server SSL certificate.Use with --use-vault
      --vault-client-cert string       ClientCert is the path to the certificate for Vault communication.Use with --use-vault
      --vault-client-key string        ClientKey is the path to the private key for Vault communication.Use with --use-vault
      --vault-path-prefix string       The Secrets Engine to which Vault should route traffic. (default "secret")
      --vault-root-key string          key prefix for Vault key-value storage inside a storage engine. (default "gloo")
      --vault-tls-insecure             Insecure enables or disables SSL verification.Use with --use-vault
      --vault-tls-server-name string   TLSServerName, if set, is used to set the SNI host when connecting via TLS.Use with --use-vault
      --vault-token string             The root token to authenticate with a Vault server. Use with --use-vault
```

### SEE ALSO

* [glooctl create secret](../glooctl_create_secret)	 - Create a secret

