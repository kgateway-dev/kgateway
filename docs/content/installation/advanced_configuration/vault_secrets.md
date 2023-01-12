---
title: Storing Gloo Edge secrets in HashiCorp Vault
weight: 50
description: Using HashiCorp Vault as a backing store for Gloo Edge secrets
---

Use [HashiCorp Vault Key-Value storage](https://www.vaultproject.io/docs/secrets/kv/kv-v2.html) as a backing store for Gloo Edge secrets.

When Gloo Edge boots, it reads a {{< protobuf name="gloo.solo.io.Settings">}} resource from a preconfigured location. By default, Gloo Edge attempts to read a `gloo.solo.io/v1.Settings` Custom Resource named `default` in the `gloo-system` namespace of your Kubernetes cluster. By editing this settings file, you can configure Vault as the secret store for your Edge environment.

{{% notice tip %}}
Want to use Vault with Gloo Edge outside of Kubernetes instead? You can provide your settings file to Gloo Edge inside of a configuration directory when you [run Gloo Edge locally]({{< versioned_link_path fromRoot="/installation/gateway/development/docker-compose-file">}}).
{{% /notice %}}

## Customizing the Gloo Edge settings file

Edit the `default` settings resource so Gloo Edge reads and writes secrets using HashiCorp Vault.

1. Edit the `default` settings resource.
   ```shell script
   kubectl --namespace gloo-system edit settings default
   ```

2. Make the following changes to the resource.
   * Remove the existing `kubernetesSecretSource` or `directorySecretSource` field, which is required for the Vault secret storage to be used.
   * Add the `vaultSecretSource` section to enable secrets to be read from and written to Vault.
   * Add the `refreshRate` field, which is used for watching Vault secrets and the local filesystem for changes.
   {{< highlight yaml "hl_lines=16-25" >}}
   apiVersion: gloo.solo.io/v1
   kind: Settings
   metadata:
     name: default
     namespace: gloo-system
   spec:
     discoveryNamespace: gloo-system
     gateway:
       validation:
         alwaysAccept: true
         proxyValidationServerAddr: gloo:9988
     gloo:
       xdsBindAddr: 0.0.0.0:9977
     kubernetesArtifactSource: {}
     kubernetesConfigSource: {}
     # Delete or comment out the existing 
     # kubernetesSecretSource or directorySecretSource field
     #kubernetesSecretSource: {}
     # Enable secrets to be read from and written to HashiCorp Vault
     vaultSecretSource:
       address: http://vault:8200
       token: root
     # refresh rate for polling config backends for changes
     # this is used for watching vault secrets and the local filesystem
     refreshRate: 15s
     requestTimeout: 0.5s
   {{< /highlight >}}

For the full list of options for Gloo Edge Settings, including the ability to set auth/TLS parameters for Vault, see the {{< protobuf name="gloo.solo.io.Settings" display="v1.Settings API reference">}}.

## Writing secret objects to Vault

After configuring Vault as your secret store, be sure to write any Vault secrets by using Gloo Edge-style YAML. You can either use the `glooctl create secret` command or manually write secrets.

### Using glooctl

To get started writing Gloo Edge secrets for use with Vault, you can use the `glooctl create secret` command. A benefit of using `glooctl` for secret creation is that the secret is created in the path that Gloo Edge watches.

For example, you might use the following command to create a secret in Vault.
```bash
glooctl create secret tls \
    --certchain /path/to/cert.pem \
    --privatekey /path/to/key.pem
    --name tls-secret \
    --use-vault \
    --vault-address http://vault:8200/ \
    --vault-token "$VAULT_TOKEN"
```
This command creates a TLS secret with the following value:
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

You can also include the `-o json` flag in the command for JSON-formatted secrets, which can be manually stored as values in Vault.

### Manually writing secrets

Be sure to write any Vault secrets by using Gloo Edge-style YAML. For more information, see the {{< protobuf name="gloo.solo.io.Secret" display="v1.Secret API reference">}}.

If you manually write Gloo Edge secrets, you must store them in Vault with the correct Vault key names, which adhere to the following format:

`<secret_engine_path_prefix>/<gloo_root_key>/<resource_group>/<group_version>/Secret/<resource_namespace>/<resource_name>`

| Path | Description |
| ---- | ----------- |
| `<secret_engine_path_prefix>` | The `pathPrefix` configured in the Settings `vaultSecretSource`. Defaults to `secret`. Note that the default path for the kv secrets engine in Vault is `kv`. |
| `<gloo_root_key>` | The `rootKey` configured in the Settings `vaultSecretSource`. Defaults to `gloo` |
| `<resource_group>` | The API group/proto package in which resources of the given type are contained. {{< protobuf name="gloo.solo.io.Secret" display="Gloo Edge secrets">}} have the resource group `gloo.solo.io`. |
| `<group_version>` | The API group version/go package in which resources of the given type are contained. For example, {{< protobuf name="gloo.solo.io.Secret" display="Gloo Edge secrets">}} have the resource group version `v1`. |
| `<resource_namespace>` | The namespace in which the secret exists. This must match the `metadata.namespace` of the resource YAML. |
| `<resource_name>` | The name of the secret. This must match the `metadata.name` of the resource YAML, and should be unique for all secrets within a given namespace. |