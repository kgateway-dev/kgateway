# Install Gloo using Docker-Compose

1. Clone the gloo repository, and cd to this example: `git clone https://github.com/solo-io/gloo && cd gloo/docs/getting_started/docker-compose`
1. Run `./prepare-config-directories.sh`
1. You can optionally set GLOO_VERSION environment variable to the gloo version you want (defaults to "0.6.19").
1. Run `docker compose up`

# Example

This configuration comes pre-loaded with an example upstream:

```
# view the upstream definition
cat data/config/upstreams/gloo-system/petstore.yaml

metadata:
  name: petstore
  namespace: gloo-system
upstream_spec:
  static:
    hosts:
    - addr: petstore
      port: 8080

# gloo will automatically discover functions (may take a few seconds)

cat data/config/upstreams/gloo-system/petstore.yaml

metadata:
  name: petstore
  namespace: gloo-system
  resourceVersion: "4"
status:
  reportedBy: gloo
  state: Accepted
upstreamSpec:
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

# see how the route is configured:
cat data/config/virtualservices/gloo-system/default.yaml

metadata:
  name: default
  namespace: gloo-system
  resourceVersion: "2"
status:
  reportedBy: gateway
  state: Accepted
  subresourceStatuses:
    '*v1.Proxy gloo-system gateway-proxy':
      reportedBy: gloo
      state: Accepted
virtualHost:
  name: gloo-system.default
  routes:
  - matcher:
      exact: /petstore/findPet
    routeAction:
      single:
        destinationSpec:
          rest:
            functionName: findPetById
        upstream:
          name: petstore
          namespace: gloo-system

# try the route
curl localhost:8080/petstore/findPet
```