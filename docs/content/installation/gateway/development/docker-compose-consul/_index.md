---
title: Run Gloo Edge Locally with Hashicorp Consul & Vault
menuTitle: Consul & Vault
weight: 10
description: How to run Gloo Edge Locally using Docker Compose with HashiCorp Consul & Vault
---

While Gloo Edge is typically run on Kubernetes, it doesn't need to be! You can run Gloo Edge using Docker Compose on your local machine.

Kubernetes provides APIs for config storage ([CRDs](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)), credential storage ([Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)), and service discovery ([Services](https://kubernetes.io/docs/concepts/services-networking/service/)). These APIs need to be substituted with another option when Gloo Edge is not running on Kubernetes.

Fortunately, Gloo Edge provides alternate mechanisms for configuration, credential storage, and service discovery that do not require Kubernetes, including the use of local `.yaml` files, [Consul Key-Value storage](https://www.consul.io/api/kv.html) and [Vault Key-Value storage](https://www.vaultproject.io/docs/secrets/kv/kv-v2.html).

This tutorial provides a basic installation flow for running Gloo Edge with Docker Compose, connecting it to Consul for configuration storage, and using Vault for credential storage.

First we will copy the necessary files from the [Solo.io GitHub](https://github.com/solo-io/gloo) repository. 

Then we will use `docker-compose` to create the containers for Gloo Edge, Consul, Vault, and the Pet Store application.

Once the containers are up and running, we will create an *Upstream* for the Pet Store application and a *Virtual Service* on Gloo Edge to route requests from the gateway to the Pet Store application.

Finally, we will observe the entries made in Consul and validate the routing rule on the *Virtual Service* is working.

{{% notice note %}}
The deployment steps in this tutorial should be modified for production environments. This tutorial is meant as an example of how to configure Gloo Edge to connect to Consul and Vault for configuration/secrets. In a production environment, additional steps should be taken to ensure Gloo Edge has the proper ACL tokens to communicate with production Consul and Vault clusters.
{{% /notice %}}

---

## Architecture

Gloo Edge without Kubernetes uses multiple pieces of software for deployment and functionality.

- **Docker Compose**: The components of Gloo Edge are deployed as containers running discovery, proxy, envoy, and the gateway
- **Consul**: Consul is used to store key/value pairs that represent the configuration of Gloo Edge
- **Vault**: Vault is used to house sensitive data used by Gloo Edge
- **Glooctl**: Command line tool for installing and configuring Gloo Edge

## Preparing for Installation

Before proceeding to the installation, you will need to complete some prerequisites.

### Prerequisite Software

Installation on your local system requires the following applications to be installed.

- [Docker](https://docs.docker.com/)
- [Docker Compose](https://docs.docker.com/compose/install/)
- [Glooctl](https://github.com/solo-io/gloo/releases)

### Download the Installation Files

This tutorial uses files stored on the [Gloo Edge GitHub repository](https://github.com/solo-io/gloo).

In order to install Gloo Edge using Docker-Compose, let's clone the repository:

```
git clone https://github.com/solo-io/gloo
cd gloo/install/docker-compose-consul
```

The files used for installation live in the `install/docker-compose-consul` directory.

```bash
├── source_data
│   ├── envoy-config.yaml
│   ├── gloo-system
│   │   └── default.yaml
│   └── gateways
│       └── gloo-system
│           └── gw-proxy.yaml
├── docker-compose.yaml
└── prepare-directories.sh
```

Now we are ready to deploy the containers using Docker Compose.

---

## Deploying with Docker Compose

Now that we have all the necessary files, it is time to deploy the containers using Docker Compose. The `docker-compose.yaml` file will create seven containers: `consul`, `vault`, `petstore`, `gloo`, `discovery`, and `gateway-proxy`.

First we need to create some directories that will be used by the Gloo Edge containers. Running the `prepare-directories.sh` script will create the necessary directory structure in the `data` directory.

```bash
./prepare-directories.sh
```

Next let's run `docker-compose up` from the `docker-compose-consul` directory to start up the containers. The version of Consul, Vault, and Gloo Edge can be controlled using the environment variables `CONSUL_VERSION`, `VAULT_VERSION`, and `GLOO_VERSION`. It's probably best to stick with the defaults, unless you have a compelling reason to change them.

```bash
docker-compose up
```

The following ports will be exposed to the host machine:

|  service  | port |
| ----- | ---- |  
| consul | 8500 | 
| vault | 8200 | 
| gloo/http | 8080 | 
| petstore | 8090 |
| gloo/https | 8443 | 
| gloo/admin | 19000 | 

You can view resources stored in the Consul UI at [http://localhost:8500/ui](http://localhost:8500/ui).

You can also view secrets stored in the Vault UI at [http://localhost:8200/ui](http://localhost:8200/ui). Use the `Token` sign-in method, with `root` as the token.

With all the containers now running, it is time to configure the *Upstream* for the Per Store application and a *Virtual Service* on the Gloo Edge gateway to serve content from the Pet Store app.

---

## Configuring the Gateway

From the repo root:

```shell script
curl --request PUT --data-binary @./install/docker-compose-consul/data/gateways/gloo-system/gw-proxy.yaml http://127.0.0.1:8500/v1/kv/gloo/gateway.solo.io/v1/Gateway/gloo-system
```

## Configuring Upstream and Virtual Services

The next step is to expose the Pet Store's API through the Gloo Edge gateway. We will do this by creating a service on Consul that Gloo Edge will use as an *Upstream*. Then we will create a *Virtual Service* on Gloo Edge with a routing rule. The configuration data for the *Virtual Service* will be stored in Consul.

To create the service on Consul, we need to get the IP address of the `petstore` container. The command below retrieves the IP address and then creates a JSON file with information about the Pet Store application. The JSON file will be submitted to Consul to create the service.

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

Now that we have the JSON file for the Pet Store application, let's register the `petstore` service with Consul using `curl`:

```bash
curl -v \
    -XPUT \
    --data @petstore-service.json \
    "http://127.0.0.1:8500/v1/agent/service/register"
```

Looking at the Consul UI under Services, we should see two services registered. The `consul` service and the `petstore` service.

![Consul UI Services]({{% versioned_link_path fromRoot="/img/docker-compose-consul-services.png" %}})

The `petstore` service can be used as an *Upstream* destination by a *Virtual Service* definition on Gloo Edge. Let’s now use `glooctl` to create a basic route for this upstream with the `--prefix-rewrite` flag to rewrite the path on incoming requests to match the path our petstore application expects. The `--use-consul` flag indicates to Gloo Edge that it will be using Consul to store this configuration and not Kubernetes.

```bash
glooctl add route \
    --path-exact /all-pets \
    --dest-name petstore \
    --prefix-rewrite /api/pets \
    --use-consul
```

```console
+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+
| VIRTUAL SERVICE | DISPLAY NAME | DOMAINS | SSL  | STATUS  | LISTENERPLUGINS |             ROUTES             |
+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+
| default         |              | *       | none | Pending |                 | /all-pets ->                   |
|                 |              |         |      |         |                 | gloo-system.petstore           |
|                 |              |         |      |         |                 | (upstream)                     |
+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+
```

Looking in the Consul UI, we can drill down on the K/V store to find the configuration stored at `gloo/gateway.solo.io/v1/VirtualService/gloo-system/default`.

```yaml
metadata:
  name: default
  namespace: gloo-system
  resourceVersion: "20"
status:
  reportedBy: gateway
  state: Accepted
  subresourceStatuses:
    '*v1.Proxy.gloo-system.gateway-proxy':
      reportedBy: gloo
      state: Accepted
virtualHost:
  domains:
  - '*'
  routes:
  - matchers:
    - exact: /all-pets
    options:
      prefixRewrite: /api/pets
    routeAction:
      single:
        upstream:
          name: petstore
          namespace: gloo-system
```

We should now be able to send a request to the Gloo Edge proxy on the path `/all-pets` and retrieve a result from the Pet Store application on the path `/api/pets`. Let's use `curl` to send a request:

```bash
curl http://localhost:8080/all-pets
```

The response should look like the JSON payload shown below.

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

---

## Next Steps

Congratulations! You've successfully deployed Gloo Edge with Docker Compose and created your first route. Now let's delve deeper into the world of [Traffic Management with Gloo Edge]({{< versioned_link_path fromRoot="/guides/traffic_management/" >}}). 

Most of the existing tutorials for Gloo Edge use Kubernetes as the underlying resource, but they can also use a Docker Compose deployment. Remember that all `glooctl` commands should be used with the `--use-consul` flag, and deployments will need to be orchestrated through Docker Compose.
