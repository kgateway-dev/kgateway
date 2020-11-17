---
title: Storing Gloo Edge Config in Consul
weight: 50
description: Using Consul as a backing store for Gloo Edge configuration
---

While Kubernetes provides APIs for config storage ([CRDs](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)), credential storage ([Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)), and service discovery ([Services](https://kubernetes.io/docs/concepts/services-networking/service/)), users may wish to run Gloo Edge without using Kubernetes.

Gloo Edge provides alternate mechanisms for configuration, credential storage, and service discovery that do not require Kubernetes, including the use of local `.yaml` files, [Consul Key-Value storage](https://www.consul.io/api/kv.html) and [Vault Key-Value storage](https://www.vaultproject.io/docs/secrets/kv/kv-v2.html).

This document describes how to write configuration YAML to Consul's Key-Value store to configure Gloo Edge.

---

## Configuring Gloo Edge using custom Settings

When Gloo Edge boots, it attempts to read a {{< protobuf name="gloo.solo.io.Settings">}} resource from a preconfigured location. By default, Gloo Edge will attempt to connect to a Kubernetes cluster and look up the `gloo.solo.io/v1.Settings` Custom Resource in namespace `gloo-system`, named `default`. 

When desiring to run without Kubernetes, it is possible to instead provide this file to Gloo Edge inside of a configuration directory.

When running the `gloo`, `discovery`, and `gateway` processes, it is necessary to provide a `--dir` flag pointing to the config directory containing the Settings YAML.

If we were to create a directory called `data`, the structure of the directory should look like so:

```bash
data
├── artifacts
├── gloo-system
│   └── default.yaml
└── secrets

3 directories, 1 file
```

When we pass the flag `--dir=./data` to Gloo Edge, Gloo Edge will look for the settings file in `data/<namespace>/*.yaml`. The default namespace for Gloo Edge is `gloo-system`. This can be overridden with the `--namespace` flag.

### Customizing the Gloo Edge Settings file

The full list of options for Gloo Edge Settings, including the ability to set auth/TLS parameters for Consul can be found {{< protobuf name="gloo.solo.io.Settings" display="in the v1.Settings API reference">}}.

Here is provided an example Settings so Gloo Edge will read config from Consul Key-Value store:

{{< highlight yaml "hl_lines=12-17" >}}
# metadata of the Settings resource contained in this file
# name should always be set to default
# namespace should be "gloo-system" or the value of the --namespace used to start Gloo Edge
metadata:
  name: default
  namespace: gloo-system

# bind address for gloo's configuration server
gloo:
  xdsBindAddr: 0.0.0.0:9977

# connection options for consul
consul:
  # address of the consul agent/server
  address: 127.0.0.1:8500
  # enable service discovery using consul
  serviceDiscovery: {}

# enable configuration using consul key-value storage
consulKvSource: {}

# enable secrets to be read from the local filesystem
directorySecretSource:
  directory: /data/secret

# currently unused, but required
# /data/artifacts will be used
# for large file storage
directoryArtifactSource:
  directory: /data

# the namespace/parent directory
# to which to write discovered resources, such as upstreams
discoveryNamespace: gloo-system

# refresh rate for polling config backends for changes
# this is used for watching vault secrets and the local filesystem
refreshRate: 15s

# status will be reported by Gloo Edge as "Accepted"
# if booted successfully
status: {}

{{< /highlight >}}

---

## Writing Config Objects to Consul

Consul Values should be written using Gloo Edge-style YAML, whose structure is described in the [`API Reference`]({{< versioned_link_path fromRoot="/reference/api" >}}).

`glooctl` provides a convenience to get started writing Gloo Edge resources for use with Consul.

Using `glooctl add route ... -o yaml` and `glooctl create ... -o yaml` will output YAML-formatted objects which can be stored as values in Consul.

For example:

```bash
glooctl add route \
    --path-exact /sample-route-1 \
    --dest-name petstore \
    --prefix-rewrite /api/pets -oyaml
```

Will produce the following:

```yaml

metadata:
  name: default
  namespace: gloo-system
status: {}
virtualHost:
  domains:
  - '*'
  routes:
  - matchers:
     - exact: /sample-route-1
    routeAction:
      single:
        upstream:
          name: petstore
          namespace: gloo-system
    options:
      prefixRewrite: /api/pets
```

Gloo Edge YAML must be stored in Consul with the correct Key names.

Consul keys adhere to the following format: 

`<root key>/<resource group>/<group version>/<resource kind>/<resource namespace>/<resource name>`

Where:

- `root key`: is the `rootKey` configured in the Settings `consulKvSource`. Defaults to `gloo`
- `resource group`: is the API group/proto package in which resources of the given type are contained. For example, {{< protobuf name="gloo.solo.io.Upstream" display="Gloo Edge Upstreams">}} have the resource group `gloo.solo.io`.
- `group version`: is the API group version/go package in which resources of the given type are contained. For example, {{< protobuf name="gloo.solo.io.Upstream" display="Gloo Edge Upstreams">}} have the resource group version `v1`.
- `resource kind`: is the full name of the resource type. For example, {{< protobuf name="gloo.solo.io.Upstream" display="Gloo Edge Upstreams">}} have the resource kind `Upstream`.
- `resource namespace`: is the namespace in which the resource should live. this should match the `metadata.namespace` of the resource YAML.
- `resource name`: is the name of the given resource. this should match the `metadata.name` of the resource YAML, and should be unique for all resources of a type within a given namespace.

The paths for Gloo Edge's API objects are as follows:

| Resource | Key |
| ----- | ---- | 
| {{< protobuf name="gloo.solo.io.Upstream">}} | `gloo/gloo.solo.io/v1/Upstream/<namespace>/<name>`  |
| {{< protobuf name="gateway.solo.io.VirtualService">}} | `gloo/gateway.solo.io/v1/VirtualService/<namespace>/<name>`  |
| {{< protobuf name="gateway.solo.io.Gateway">}} | `gloo/gateway.solo.io/v1/Gateway/<namespace>/<name>`  |
| {{< protobuf name="gloo.solo.io.Proxy">}} | `gloo/gloo.solo.io/v1/Proxy/<namespace>/<name>`  |

To store a Gloo Edge resource in Consul, one can use `curl` or the `consul` CLI:

```bash
# store using curl:
curl -v \
    -XPUT \
    --data-binary "@<resource yaml file>.yaml" \
    "http://127.0.0.1:8500/v1/kv/gloo/<resource group>/<group version>/<resource kind>/<namespace>/<name>"

# store using consul:
consul kv put gloo/<resource group>/<group version>/<resource kind>/<namespace>/<name> @<resource yaml file>.yaml
```

For example, to store a Virtual Service:

```bash
# store using curl:
curl -v \
    -XPUT \
    --data-binary "@virtual-service.yaml" \
    "http://127.0.0.1:8500/v1/kv/gloo/gateway.solo.io/v1/VirtualService/gloo-system/default"

# store using consul:
consul kv put gloo/gateway.solo.io/v1/VirtualService/gloo-system/default @virtual-service.yaml
```

Stored resources can be viewed via the consul UI:

![Consul UI]({{% versioned_link_path fromRoot="/img/consul_virtual_service.png" %}} "Consul Virtual Service")

This can be useful for modifying configuration, or viewing the status reported by Gloo Edge.
