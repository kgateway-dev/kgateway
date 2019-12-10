---
title: Run Gloo Gateway Locally
weight: 5
description: How to run Gloo Locally using Docker-Compose
---

While Gloo is typically run on Kubernetes, it doesn't need to be! You can run Gloo using Docker Compose on your local machine.

Kubernetes provides APIs for config storage ([CRDs](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)), credential storage ([Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)), and service discovery ([Services](https://kubernetes.io/docs/concepts/services-networking/service/)). These APIs need to be substituted with another option when Gloo is not running on Kubernetes.

Fortunately, Gloo provides alternate mechanisms for configuration, credential storage, and service discovery that do not require Kubernetes, including the use of local `.yaml` files, [Consul Key-Value storage](https://www.consul.io/api/kv.html) and [Vault Key-Value storage](https://www.vaultproject.io/docs/secrets/kv/kv-v2.html).

This tutorial provides a basic installation flow for running Gloo with Docker Compose, using the local filesystem of the containers to store configuration and credentials data.

First we will copy the necessary files from the [Solo.io GitHub](https://github.com/solo-io/gloo) repository. 

Then we will use `docker-compose` to create the containers for Gloo and the Pet Store application.

Once the containers are up and running, we will examine the *Upstream* YAML file for the Pet Store application and the *Virtual Service* YAML file used by Gloo to route requests from the Gloo Gateway to the Pet Store application.

Finally, we will use curl to validate the routing rule on the *Virtual Service* is working.

{{% notice note %}}
The deployment steps in this tutorial are for demonstration and learning on a local machine and should not be used in a production setting. Please refer to using either Kubernetes or Consul & Vault to provide storage of configuration and credentials.
{{% /notice %}}

---

## Architecture

Gloo Gateway without Kubernetes uses multiple pieces of software for deployment and functionality.

- **Docker Compose**: The components of Gloo are deployed as containers running proxy, envoy, and the gateway
- **Glooctl**: Command line tool for installing and configuring Gloo

## Preparing for Installation

Before proceeding to the installation, you will need to complete some prerequisites.

### Prerequisite Software

Installation on your local system requires the following applications to be installed.

- [Docker](https://docs.docker.com/)
- [Docker Compose](https://docs.docker.com/compose/install/)
- [Glooctl](https://github.com/solo-io/gloo/releases)

### Download the Installation Files

This tutorial uses files stored on the [Gloo GitHub repository](https://github.com/solo-io/gloo).

In order to install Gloo using Docker-Compose, let's clone the repository:

```
git clone --branch v1.1.0 https://github.com/solo-io/gloo
cd gloo/install/docker-compose-file
```

The files used for installation live in the `install/docker-compose-file` directory.

```bash
├── data
│   ├── config
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

Since we are using the filesystem to store the Gloo configuration and credentials, we need to set up a directory structure to support that. Each of the Gloo containers created by the `docker-compose.yaml` file will attach to the `data` directory inside the `install/docker-compose-file` parent directory. 

Although much of the structure is already set up, there are some additional empty directories that must be created for use by the Gloo containers. Let's run the `prepare-directories.sh` script to create the rest.

```bash
./prepare-directories.sh
```

The updated `data` directory structure should look like this:

```console
├── artifact
│   └── artifacts
│       └── gloo-system
├── config
│   ├── gateways
│   │   └── gloo-system
│   ├── proxies
│   │   └── gloo-system
│   ├── routetables
│   │   └── gloo-system
│   ├── upstreamgroups
│   │   └── gloo-system
│   ├── upstreams
│   │   └── gloo-system
│   │       └── petstore.yaml
│   └── virtualservices
│       └── gloo-system
│           └── default.yaml
├── envoy-config.yaml
├── gloo-system
│   └── default.yaml
└── secret
    └── secrets
        ├── default
        └── gloo-system
```

The file `data/gloo-system/default.yaml` provides the initial configuration for each Gloo container, including where to store secrets, configs, and artifacts.

The file `data/config/upstreams/gloo-system/petstore.yaml` defines an *Upstream* configuration for the Pet Store application that Gloo can use as a target to route requests.

The file `data/config/virtualservice/gloo-system/default.yaml` defines a default *Virtual Service* with routing rules to send traffic from the proxy to the Pet Store *Upstream*.

Now that we have the proper directory structure in place, we can deploy the containers.

---

## Deploying with Docker Compose

With the necessary directory structure in place, it is time to deploy the containers using Docker Compose. The `docker-compose.yaml` file will create four containers: `petstore`, `gloo`, `gateway`, and `gateway-proxy`.

Let's run `docker-compose up` from the `docker-compose-file` directory to start up the containers. The version of Gloo can be controlled using the environment variable `GLOO_VERSION`. It's probably best to stick with the default version, unless you have a compelling reason to change it.

```bash
docker-compose up
```

The following ports will be exposed to the host machine:

|  service  | port |
| ----- | ---- |  
| gloo/http | 8080 | 
| petstore | 8090 |
| gloo/https | 8443 | 
| gloo/admin | 19000 | 

In addition to opening ports, there should be three new files in the `data` directory. 

* `config/gateways/gloo-system/gateway-proxy-v2-ssl.yaml`
* `config/gateways/gloo-system/gateway-proxy-v2.yaml`
* `config/proxies/gloo-system/gateway-proxy-v2.yaml`

The three files represent the configuration for the `gateway` and `gateway-proxy` containers.

The containers should have loaded their base configuration as well as the Pet Store *Upstream* and default *Virtual Service*. In the next section we will examine the contents of the two configuration files.

---

## Examining the *Upstream* and *Virtual Service*

Docker Compose created the necessary Gloo containers along with the Pet Store applicaiton. The configuration comes pre-loaded with an example *Upstream* Gloo uses to allow function level routing. Let's examine the contents of the `petstore.yaml` file.

```shell
# view the upstream definition
cat data/config/upstreams/gloo-system/petstore.yaml
```

```yaml
metadata:
  name: petstore
  namespace: gloo-system
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
              text: /api/pets?tags={{default(tags, "")}}&limit={{default(limit,
                "")}}
            content-length:
              text: "0"
            content-type: {}
            transfer-encoding: {}
```

The *Upstream* configuration defines the address where the Pet Store service can be found and the REST services that are offered by the application. Gloo can use the transformations found in the configuration as a target for routing requests.

The other half of the equation is the *Virtual Service*. Let's take a look at the `default.yaml` file that defines the default *Virtual Service* on the Gloo Gateway.

```shell
# see how the route is configured:
cat data/config/virtualservices/gloo-system/default.yaml
```

```yaml
metadata:
  name: default
  namespace: gloo-system
virtualHost:
  domains:
  - '*'
  routes:
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
     - prefix: /petstore
    routeAction:
      single:
        upstream:
          name: petstore
          namespace: gloo-system
    options:
      prefixRewrite: /api/pets
```

The default *Virtual Service* defines several possible routes to pass through to the Pet Store application. For instance, a request on the prefix `/petstore` would route that request to the `/api/pets` prefix on the Pet Store *Upstream*.

You'll need to wait a minute for the virtual service to get processed by Gloo and the routes exposed externally.

### Testing the Gloo Configuration

We should now be able to send a request to the Gloo proxy and receive a reply based on the prefix we use. Let's use `curl` to send a request:

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

Congratulations! You've successfully deployed Gloo with Docker Compose and created your first route. Now let's delve deeper into the world of [Gloo routing]({{< versioned_link_path fromRoot="/gloo_routing" >}}). 

Most of the existing tutorials for Gloo use Kubernetes as the underlying resource, but they can also use a Docker Compose deployment. It will be necessary to handcraft the proper YAML files for each configuration, so it might make more sense to check out using either Kubernetes or Consul & Vault to store configuration data.
