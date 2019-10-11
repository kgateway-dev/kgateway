---
title: "glooctl create secret aws"
weight: 5
---
## glooctl create secret aws

Create an AWS secret with the given name

### Synopsis

Create an AWS secret with the given name

```
glooctl create secret aws [flags]
```

### Options

```
      --access-key string   aws access key
  -h, --help                help for aws
      --secret-key string   aws secret key
```

### Options inherited from parent commands

```
      --dry-run                        print kubernetes-formatted yaml rather than creating or updating a resource
  -i, --interactive                    use interactive mode
      --kubeconfig string              kubeconfig to use, if not standard one
      --name string                    name of the resource to read or write
  -n, --namespace string               namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType              output format: (yaml, json, table, kube-yaml, wide) (default table)
      --use-vault                      use Vault Key-Value storage as the backend for reading and writing secrets
      --vault-address string           address of the Vault server. This should be a complete  URL such as "http://vault.example.com". Use with --use-vault (default "https://127.0.0.1:8200")
      --vault-ca-cert string           CACert is the path to a PEM-encoded CA cert file to use to verify the Vault server SSL certificate.Use with --use-vault
      --vault-ca-path string           CAPath is the path to a directory of PEM-encoded CA cert files to verify the Vault server SSL certificate.Use with --use-vault
      --vault-client-cert string       ClientCert is the path to the certificate for Vault communication.Use with --use-vault
      --vault-client-key string        ClientKey is the path to the private key for Vault communication.Use with --use-vault
      --vault-root-key string          key prefix for for Vault key-value storage. (default "gloo")
      --vault-tls-insecure             Insecure enables or disables SSL verification.Use with --use-vault
      --vault-tls-server-name string   TLSServerName, if set, is used to set the SNI host when connecting via TLS.Use with --use-vault
      --vault-token string             address of the Vault server. This should be a complete  URL such as "http://vault.example.com". Use with --use-vault
```

### SEE ALSO

* [glooctl create secret](../glooctl_create_secret)	 - Create a secret

