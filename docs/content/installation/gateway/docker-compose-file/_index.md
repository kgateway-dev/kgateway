---
title: Run Gloo Gateway Locally
weight: 5
description: How to run Gloo Locally using Docker-Compose
---

1. Clone the solo-docs repository and cd to this example: `git clone https://github.com/solo-io/solo-docs && cd solo-docs/gloo/docs/installation/gateway/docker-compose-file`
1. Run `./prepare-directories.sh`
1. You can optionally set `GLOO_VERSION` environment variable to the Gloo version you want (defaults to "0.17.4").
1. Run `docker-compose up`

## Example

This configuration comes pre-loaded with an example upstream that includes an optional function service specification `serviceSpec`
that Gloo uses to allow function level routing.

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
  - matcher:
      prefix: /petstore/findWithId
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
  - matcher:
      prefix: /petstore/findPets
    routeAction:
      single:
        destinationSpec:
          rest:
            functionName: findPets
            parameters: {}
        upstream:
          name: petstore
          namespace: gloo-system
  - matcher:
      prefix: /petstore
    routeAction:
      single:
        upstream:
          name: petstore
          namespace: gloo-system
    routePlugins:
      prefixRewrite:
        prefixRewrite: /api/pets
```

You'll need to wait a minute for the virtual service to get processed by Gloo and the routes exposed externally.

```shell
# try the routes
curl http://localhost:8080/petstore/findWithId/1
curl http://localhost:8080/petstore/findPets
curl http://localhost:8080/petstore/
```
