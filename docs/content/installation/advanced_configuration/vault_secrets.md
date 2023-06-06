---
title: Storing Gloo Edge secrets in HashiCorp Vault
weight: 50
description: Using HashiCorp Vault as a backing store for Gloo Edge secrets
---

Use [HashiCorp Vault Key-Value storage](https://www.vaultproject.io/docs/secrets/kv/kv-v2.html) as a backing store for Gloo Edge secrets.

When Gloo Edge boots, it reads the {{< protobuf name="gloo.solo.io.Settings">}} custom resource named `default` in the `gloo-system` namespace of your Kubernetes cluster to receive the proxy configuration for the gateway. By default, this configuration directs Gloo Edge to connect to and use Kubernetes as the secret store for your environment. If you want Gloo Edge to read and write secrets using a HashiCorp Vault instance instead of storing secrets directly in your Kubernetes cluster, you can edit the Settings custom resource to point the gateway proxy to Vault.

{{% notice tip %}}
Want to use Vault with Gloo Edge outside of Kubernetes instead? You can provide your settings file to Gloo Edge inside of a configuration directory when you [run Gloo Edge locally]({{< versioned_link_path fromRoot="/installation/gateway/development/docker-compose-file">}})
{{% /notice %}}

## Customizing the Gloo Edge settings file

Edit the `default` settings resource so Gloo Edge reads and writes secrets using HashiCorp Vault.

**Before you begin**: [Set up a Vault instance](https://developer.hashicorp.com/vault/tutorials/getting-started/getting-started-install) either in your Gloo Edge cluster or externally. Your instance must be routable on an address that you provide to Gloo Edge in the following steps, such as `http://vault:8200`.

1. Edit the `default` settings resource.
   ```shell script
   kubectl --namespace gloo-system edit settings default
   ```

2. Make the following changes to the resource.
   * Remove the existing `kubernetesSecretSource`, `vaultSecretSource`, or `directorySecretSource` field, which directs the gateway to use secret stores other than Vault.
   * Add the `secretOptions` section with a Kubernetes source and a Vault source specified to enable secrets to be read from both Kubernetes and Vault.
   * Add the `refreshRate` field to watch for changes in Vault secrets and the local filesystem of where Gloo Edge runs.
   {{< highlight yaml "hl_lines=16-27" >}}
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
     # Delete or comment out the existing \*SecretSource field
     #kubernetesSecretSource: {}
     secretOptions:
       sources:
       # Enable secrets to be read from and written to HashiCorp Vault
       - vault:
         # Add the address that your Vault instance is routeable on
         address: http://vault:8200
         accessToken: root
     # Add the refresh rate for polling config backends for changes
     # This setting is used for watching vault secrets and by other resource clients
     refreshRate: 15s
     requestTimeout: 0.5s
   {{< /highlight >}}

For the full list of options for Gloo Edge Settings, including the ability to set auth/TLS parameters for Vault, see the {{< protobuf name="gloo.solo.io.Settings" display="v1.Settings API reference">}}.

An example using AWS IAM auth might look like the following:
   {{< highlight yaml "hl_lines=16-30" >}}
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
     # Delete or comment out the existing \*SecretSource field
     #kubernetesSecretSource: {}
     secretOptions:
       sources:
       # Enable secrets to be read from and written to HashiCorp Vault
       - vault:
           # Address that your Vault instance is routeable on
           address: http://vault:8200
           aws:
             vaultRole: vault-role
             region: us-east-1
             iamServerIdHeader: vault.example.com
             accessKeyId: your-aws-iam-access-key-id
             secretAccessKey: your-aws-iam-secret-access-key
             sessionToken: your-aws-iam-session-token
     # refresh rate for polling config backends for changes
     # this is used for watching vault secrets and by other resource clients
     refreshRate: 15s
     requestTimeout: 0.5s
   {{< /highlight >}}


## Writing secret objects to Vault

After configuring Vault as your secret store, be sure to write any Vault secrets by using Gloo Edge-style YAML. You can either use the `glooctl create secret` command or manually write secrets.

### Using glooctl

To get started writing Gloo Edge secrets for use with Vault, you can use the `glooctl create secret` command. A benefit of using `glooctl` for secret creation is that the secret is created in the path that Gloo Edge watches, `secret/root/gloo.solo.io/v1/gloo-system/tls-secret`.

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

Instead of using the `glooctl create secret` command to create the Vault secret and automatically store the secret key in your Vault instance, you can use your own configuration file to create the secret. Note that you must use the same YAML format for the secret so that Gloo Edge can read the secret. For more information, see the {{< protobuf name="gloo.solo.io.Secret" display="v1.Secret API reference">}}.

If you manually write Gloo Edge secrets, you must store them in Vault with the correct Vault key names, which adhere to the following format:

`<secret_engine_path_prefix>/<gloo_root_key>/<resource_group>/<group_version>/Secret/<resource_namespace>/<resource_name>`

For example, if you want to create a secret named `tls-secret` in the `gloo-system` namespace, store the secret file in Vault on the path `secret/root/gloo.solo.io/v1/Secret/gloo-system/tls-secret`.

| Path | Description |
| ---- | ----------- |
| `<secret_engine_path_prefix>` | The `pathPrefix` configured in the Settings `vaultSecretSource`. Defaults to `secret`. Note that the default path for the kv secrets engine in Vault is `kv` when Vault is not run with `-dev`. |
| `<gloo_root_key>` | The `rootKey` configured in the Settings `vaultSecretSource`. Defaults to `gloo` |
| `<resource_group>` | The API group/proto package in which resources of the given type are contained. The {{< protobuf name="gloo.solo.io.Secret" display="Gloo Edge secrets">}} custom resource has the resource group `gloo.solo.io`. |
| `<group_version>` | The API group version/go package in which resources of the given type are contained. For example, {{< protobuf name="gloo.solo.io.Secret" display="Gloo Edge secrets">}} have the resource group version `v1`. |
| `<resource_namespace>` | The namespace in which the secret exists. This must match the `metadata.namespace` of the resource YAML. |
| `<resource_name>` | The name of the secret. This must match the `metadata.name` of the resource YAML, and should be unique for all secrets within a given namespace. |

{{% notice tip %}}
You can also use the `--dry-run` flag in the [`glooctl secret create` command](#using-glooctl) to generate your secret in a YAML file.
{{% /notice %}}
