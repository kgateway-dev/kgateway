---
title: Run Gloo Edge Locally
menuTitle: Local Files
weight: 20
description: How to run Gloo Edge Locally using Docker-Compose
---

While Gloo Edge is typically run on Kubernetes, it doesn't need to be! You can run Gloo Edge on your local machine using Docker Compose.

Kubernetes provides APIs for config storage ([CRDs](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)), credential storage ([Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)), and service discovery ([Services](https://kubernetes.io/docs/concepts/services-networking/service/)). These APIs need to be substituted with another option when Gloo Edge is not running on Kubernetes.

Fortunately, Gloo Edge provides alternate mechanisms for configuration, credential storage, and service discovery that do not require Kubernetes, including the use of local `.yaml` files, [HashiCorp Consul Key-Value storage](https://www.consul.io/api/kv.html) and [HashiCorp Vault Key-Value storage](https://www.vaultproject.io/docs/secrets/kv/kv-v2.html).

This tutorial provides a basic installation flow for running Gloo Edge with Docker Compose, using the local filesystem of the containers to store configuration and credentials data.
(A similar tutorial using Consul and Vault instead of the local filesystem can be found [here]({{< versioned_link_path fromRoot="/installation/gateway/development/docker-compose-consul/" >}}).)

First we will copy the necessary files from the [Gloo Edge GitHub repository](https://github.com/solo-io/gloo).

Then we will use `docker-compose` to create the containers for Gloo Edge and the Pet Store application.

Once the containers are up and running, we will examine the *Upstream* YAML file for the Pet Store application and the *Virtual Service* YAML file used by Gloo Edge to route requests from the Gateway to the Pet Store application.

Finally, we will use curl to validate the routing rule on the *Virtual Service* is working.

{{% notice note %}}
The deployment steps in this tutorial are for demonstration and learning on a local machine.
If you are interested in running a production deployment of Gloo Edge using flat files, you will likely want to change the deployment architecture from how it is described in this guide.
If you are an enterprise customer, please contact us for assistance!
{{% /notice %}}

---

## Architecture

Gloo Edge without Kubernetes uses multiple pieces of software for deployment and functionality.

- **Docker Compose**: The components of Gloo Edge are deployed as containers running Gloo Edge, Envoy, and the Gateway
- **File System**: The local filesystem of the machine is used to store configuration data

## Preparing for Installation

Before proceeding to the installation, you will need to complete some prerequisites.

### Prerequisite Software

Installation on your local system requires the following applications to be installed.

- [Docker](https://docs.docker.com/)
- [Docker Compose](https://docs.docker.com/compose/install/)

### Download the Installation Files

This tutorial uses files stored on the [Gloo Edge GitHub repository](https://github.com/solo-io/gloo).

In order to install Gloo Edge using Docker-Compose, let's clone the repository:

```
git clone --branch main https://github.com/solo-io/gloo
cd gloo/install/docker-compose-file
```

The files used for installation live in the `install/docker-compose-file` directory.

```bash
├── source_data
│   ├── config
│   │   ├── gateways
│   │   │   └── gloo-system
│   │   │       └── gateway-proxy.yaml
│   │   ├── upstreams
│   │   │   └── gloo-system
│   │   │       └── petstore.yaml
│   │   └── virtualservices
│   │       └── gloo-system
│   │           └── default.yaml
│   ├── envoy-config.yaml
│   ├── gloo-system
│   │   └── default.yaml
├── docker-compose.yaml
└── prepare-directories.sh
```

### Prepare the Directory Structure

Since we are using the filesystem to store the Gloo Edge configuration and credentials, we need to set up a directory structure to support that. Each of the Gloo Edge containers created by the `docker-compose.yaml` file will attach to the `data` directory inside the `install/docker-compose-file` parent directory.

Let's run the `prepare-directories.sh` script to create the necessary configuration in the `data` directory.

```bash
./prepare-directories.sh
```

The updated `data` directory structure should look like this:

```console
├── artifact
│   └── artifacts
│       └── gloo-system
├── config
│   ├── authconfigs
│   │   └── gloo-system
│   ├── gateways
│   │   └── gloo-system
│   │       └── gateway-proxy.yaml
│   ├── graphqlapis
│   │   └── gloo-system
│   ├── proxies
│   │   └── gloo-system
│   ├── ratelimitconfigs
│   │   └── gloo-system
│   ├── routeoptions
│   │   └── gloo-system
│   ├── routetables
│   │   └── gloo-system
│   ├── upstreamgroups
│   │   └── gloo-system
│   ├── upstreams
│   │   └── gloo-system
│   │       └── petstore.yaml
│   └── virtualhostoptions
│   │   └── gloo-system
│   └── virtualservices
│       └── gloo-system
│           └── default.yaml
├── envoy-config.yaml
├── gloo-system
│   ├── default.yaml
└── secret
    └── secrets
        ├── default
        └── gloo-system

```

* `data/gloo-system/default.yaml` provides the initial configuration for the Gloo Edge and Gateway containers, including where to store secrets, configs, and artifacts.

* `data/config/gateways/gloo-system/gateway-proxy.yaml` defines additional configuration for the Gateway container.

* `data/config/upstreams/gloo-system/petstore.yaml` defines an *Upstream* configuration for the Pet Store application that Gloo Edge can use as a target to route requests.

* `data/config/virtualservices/gloo-system/default.yaml` defines a default *Virtual Service* with routing rules to send traffic from the proxy to the Pet Store *Upstream*.

* `data/envoy-config.yaml` defines the configuration for the Envoy container.

Now that we have the proper directory structure in place, we can deploy the containers.

---

## Deploying with Docker Compose

With the necessary directory structure in place, it is time to deploy the containers using Docker Compose. The `docker-compose.yaml` file will create three containers: `petstore`, `gloo`, and `gateway-proxy`.

Let's run `docker-compose up` from the `docker-compose-file` directory to start up the containers. The version of Gloo Edge can be controlled using the environment variable `GLOO_VERSION`. It's probably best to stick with the default version, unless you have a compelling reason to change it.

```bash
docker-compose up
```

The following ports will be exposed to the host machine:

| service    | port  |
|------------|-------|
| gloo/http  | 8080  |
| petstore   | 8090  |
| gloo/https | 8443  |
| gloo/admin | 19000 |
| gloo/dev   | 10010 |

In addition to opening ports, there should be a new file in the `data` directory.

* `config/proxies/gloo-system/gateway-proxy.yaml`

This file and `config/gateways/gloo-system/gateway-proxy.yaml` represent the configuration for the `gateway` and `gateway-proxy` containers.

The containers should have loaded their base configuration as well as the Pet Store *Upstream* and default *Virtual Service*. In the next section we will examine the contents of the two configuration files.

---

## Examining the *Upstream* and *Virtual Service*

Docker Compose created the necessary Gloo Edge containers along with the Pet Store application. The configuration comes pre-loaded with an example *Upstream* Gloo Edge uses to allow function level routing. Let's examine the contents of the `petstore.yaml` file.

```shell
# view the upstream definition
cat data/config/upstreams/gloo-system/petstore.yaml
```

```yaml
metadata:
  name: petstore
  namespace: gloo-system
  resourceVersion: "3"
static:
  hosts:
  - addr: petstore
    port: 8080
  serviceSpec:
    rest:
      swaggerInfo:
        url: http://petstore:8080/swagger.json
      transformations:
        addPet:
          body:
            text: '{"id": {{ default(id, "") }},"name": "{{ default(name, "")}}","tag":
              "{{ default(tag, "")}}"}'
          headers:
            :method:
              text: POST
            :path:
              text: /api/pets
            content-type:
              text: application/json
        deletePet:
          headers:
            :method:
              text: DELETE
            :path:
              text: /api/pets/{{ default(id, "") }}
            content-type:
              text: application/json
        findPetById:
          body: {}
          headers:
            :method:
              text: GET
            :path:
              text: /api/pets/{{ default(id, "") }}
            content-length:
              text: "0"
            content-type: {}
            transfer-encoding: {}
        findPets:
          body: {}
          headers:
            :method:
              text: GET
            :path:
              text: /api/pets?tags={{default(tags, "")}}&limit={{default(limit, "")}}
            content-length:
              text: "0"
            content-type: {}
            transfer-encoding: {}
```

The *Upstream* configuration defines the address where the Pet Store service can be found and the REST services that are offered by the application. Gloo Edge can use the transformations found in the configuration as a target for routing requests.

The other half of the equation is the *Virtual Service*. Let's take a look at the `default.yaml` file that defines the default *Virtual Service* on the Gateway.

```shell
# see how the route is configured:
cat data/config/virtualservices/gloo-system/default.yaml
```

```yaml
metadata:
  name: default
  namespace: gloo-system
  resourceVersion: "2"
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
    - exact: /petstore
    options:
      prefixRewrite: /api/pets
    routeAction:
      single:
        upstream:
          name: petstore
          namespace: gloo-system
  - matchers:
    - prefix: /petstore/findPets
    routeAction:
      single:
        destinationSpec:
          rest:
            functionName: findPets
            parameters: {}
        upstream:
          name: petstore
          namespace: gloo-system
  - matchers:
    - prefix: /petstore/findWithId
    routeAction:
      single:
        destinationSpec:
          rest:
            functionName: findPetById
            parameters:
              headers:
                :path: /petstore/findWithId/{id}
        upstream:
          name: petstore
          namespace: gloo-system
```

The default *Virtual Service* defines several possible routes to pass through to the Pet Store application. For instance, a request on the prefix `/petstore` would route that request to the `/api/pets` prefix on the Pet Store *Upstream*.

You'll need to wait a minute for the virtual service to get processed by Gloo Edge and the routes exposed externally.

### Testing the Gloo Edge Configuration

We should now be able to send a request to the Gloo Edge proxy and receive a reply based on the prefix we use. Let's use `curl` to send a request:

```bash
curl http://localhost:8080/petstore
```

The response should look like the JSON payload shown below.

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Let's try a different prefix to find a specific pet.

```bash
curl http://localhost:8080/petstore/findWithId/1
```

```json
{"id":1,"name":"Dog","status":"available"}
```

---

## Next Steps

Congratulations! You've successfully deployed Gloo Edge with Docker Compose and created your first route. Now let's delve deeper into the world of [Traffic Management with Gloo Edge]({{< versioned_link_path fromRoot="/guides/traffic_management/" >}}).

Most of the existing tutorials for Gloo Edge use Kubernetes as the underlying resource, but they can also use a Docker Compose deployment. It will be necessary to handcraft the proper YAML files for each configuration, so it might make more sense to check out using either Kubernetes or Consul & Vault to store configuration data.
