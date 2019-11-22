---
title: Run Gloo Gateway Locally with Hashicorp Consul & Vault
menuTitle: Run Gloo with Consul & Vault
weight: 5
description: How to run Gloo Locally using Docker-Compose
---

## Motivation

While Kubernetes provides APIs for config storage ([CRDs](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)), credential storage ([Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)), and service discovery ([Services](https://kubernetes.io/docs/concepts/services-networking/service/)), users may wish to run Gloo without using Kubernetes.

Gloo provides alternate mechanisms for configuration, credential storage, and service discovery that do not require Kubernetes, including the use of local `.yaml` files, [Consul Key-Value storage](https://www.consul.io/api/kv.html) and [Vault Key-Value storage](https://www.vaultproject.io/docs/secrets/kv/kv-v2.html).

This tutorial provides a basic installation flow for running Gloo with 
Docker Compose, connecting it to Consul for configuration storage and Vault for credential storage.

> Note: the deployment steps in this tutorial should be modified for production environments. This tutorial is  meant as an example of how to configure Gloo to connect to Consul and Vault for configuration/secrets. In a production environment, additional steps should be taken to ensure Gloo has the proper ACL tokens to communicate with production Consul and Vault clusters.

## Installation

1. Clone the solo-docs repository and cd to this example: `git clone https://github.com/solo-io/solo-docs && cd solo-docs/gloo/docs/installation/gateway/docker-compose-consul`
2. Run `./prepare-directories.sh`
3. You can optionally set `GLOO_VERSION` environment variable to the Gloo version you want (defaults to "0.18.3").
4. Run `docker-compose up`

{{% notice note %}}
Consul's KV interface will be exposed on `localhost:8500`, while the Gloo Gateway Proxy will be listening for HTTP on `localhost:8080`
and HTTPS on `localhost:8443`, respectively. You can view resources stored in the Consul UI at `http://localhost:8500/ui`.
{{% /notice %}}

## Example using Petstore

Get the IP of the Petstore service:

```bash
PETSTORE_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' docker-compose-consul_petstore_1)
cat > petstore-service.json <<EOF
{
  "ID": "petstore1",
  "Name": "petstore",
  "Address": "${PETSTORE_IP}",
  "Port": 8080
}
EOF
```

Register the Petstore service with consul:

```bash
curl -v \
    -XPUT \
    --data @petstore-service.json \
    "http://127.0.0.1:8500/v1/agent/service/register"
```

Generate a VirtualService definition with a route to the Petstore:

```bash
glooctl add route \
    --path-exact /sample-route-1 \
    --dest-name petstore \
    --prefix-rewrite /api/pets --yaml > virtual-service.yaml
```

We can see the output YAML here:

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

{{% notice note %}}
All `glooctl add` and `glooctl create` commands can be run with a `--yaml` flag
which will output Gloo YAML to stdout. These outputs can be stored as Consul Values
and `.yaml` files for configuring Gloo. See the {{< protobuf name="gateway.solo.io.VirtualService" display="API reference" >}}.)
for details on writing Gloo configuration YAML.
{{% /notice %}}

Store the Virtual Service in Consul's Key-Value store:

```bash
curl -v \
    -XPUT \
    --data-binary "@virtual-service.yaml" \
    "http://127.0.0.1:8500/v1/kv/gloo/gateway.solo.io/v1/VirtualService/gloo-system/default"
```

{{% notice note %}}
Note: Consul Keys for Gloo resources follow the following format: 
`gloo/<resource group>/<group version>/<resource kind>/<resource namespace>/<resource name>`. 
See [the Consul Key-Value configuration guide]({{< versioned_link_path fromRoot="/advanced_configuration/consul_kv" >}})
for more information.
{{% /notice %}}

You should now be able to hit the route we exposed with `curl`:

```bash
curl http://localhost:8080/sample-route-1
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

While the tutorials in User Guides have been written with Kubernetes in mind, most concepts will map directly to
Consul-based installations of Gloo. Please correspond with us on [our Slack channel](https://slack.solo.io/) while we work to expand our 
documentation on running Gloo without Kubernetes.
