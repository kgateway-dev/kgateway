---
title: Storing Gloo Edge Secrets in HashiCorp Vault
weight: 50
description: Using HashiCorp Vault as a backing store for Gloo Edge secrets
---

This document describes how to write Gloo Edge secrets to Vault's Key-Value store.

---

## Configuring Gloo Edge using custom Settings

When Gloo Edge boots, it attempts to read a {{< protobuf name="gloo.solo.io.Settings">}} resource from a preconfigured location. By default, Gloo Edge will attempt to connect to a Kubernetes cluster and look up the `gloo.solo.io/v1.Settings` Custom Resource in namespace `gloo-system`, named `default`.

When desiring to run without Kubernetes, it is possible to instead provide this file to Gloo Edge inside of a configuration directory. See the guide on [running gloo edge locally]({{< versioned_link_path fromRoot="/installation/gateway/development/docker-compose-file") for more information on that use case.

### Customizing the Gloo Edge Settings file

The full list of options for Gloo Edge Settings, including the ability to set auth/TLS parameters for Vault can be found {{< protobuf name="gloo.solo.io.Settings" display="in the v1.Settings API reference">}}.

Here is provided an example excerpt from a Settings resource so Gloo Edge will read and write secrets using HashiCorp Vault:

{{< highlight yaml "hl_lines=4-6" >}}
# enable secrets to be read from and written to HashiCorp Vault.
# you MUST delete the entry for kubernetesSecretSource or directorySecretSource
# in order for this to be valid.
vaultSecretSource:
  address: http://vault:8200
  token: root

# refresh rate for polling config backends for changes
# this is used for watching vault secrets and the local filesystem
refreshRate: 15s
{{< /highlight >}}

---

## Writing Secret Objects to Vault

Vault secrets should be written using Gloo Edge-style YAML, whose structure is described in the [`API Reference`]({{< versioned_link_path fromRoot="/reference/api" >}}).

`glooctl` provides a convenience to get started writing Gloo Edge secrets for use with Vault. A benefit of using `glooctl` for secret creation is that it will place the secret in the proper path which Gloo Edge is watching.

For example:

```bash
glooctl create secret tls \
    --certchain /path/to/cert.pem \
    --privatekey /path/to/key.pem
    --name tls-secret \
    --use-vault \
    --vault-address http://vault:8200/ \
    --vault-token "$VAULT_TOKEN"
```

Will create a TLS secret in Vault with value:

```json
{
  "metadata": {
    "name": "tls-secret",
    "namespace": "gloo-system"
  },
  "tls": {
    "certChain": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n",
    "privateKey": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n"
  }
}
```

Using `glooctl create secret ... -o json` will output JSON-formatted secrets which can be manually stored as values in Vault.

Gloo Edge secrets must be stored in Vault with the correct Key names.

Vault keys adhere to the following format:

`<secret engine path prefix>/<gloo root key>/<resource group>/<group version>/Secret/<resource namespace>/<resource name>`

Where:

- `secret engine path prefix`: is the `pathPrefix` configured in the Settings `vaultSecretSource`. Defaults to `secret`. Note that the default path for the kv secrets engine in Vault is `kv`.
- `gloo root key`: is the `rootKey` configured in the Settings `vaultSecretSource`. Defaults to `gloo`
- `resource group`: is the API group/proto package in which resources of the given type are contained. {{< protobuf name="gloo.solo.io.Secret" display="Gloo Edge Secrets">}} have the resource group `gloo.solo.io`.
- `group version`: is the API group version/go package in which resources of the given type are contained. For example, {{< protobuf name="gloo.solo.io.Secret" display="Gloo Edge Secrets">}} have the resource group version `v1`.
- `resource namespace`: is the namespace in which the resource should live. This should match the `metadata.namespace` of the resource YAML.
- `resource name`: is the name of the given resource. This should match the `metadata.name` of the resource YAML, and should be unique for all secrets within a given namespace.

