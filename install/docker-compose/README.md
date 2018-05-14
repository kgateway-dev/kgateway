# Install Gloo using Docker-Compose

1. Run `./prepare-config-directories.sh`
2. Run `docker compose up`
3. `glooctl` commands should be run from this directory to interact with gloo

Note: you will want to manually register your upstreams with `glooctl`
(using `glooctl upstream create`). Their **Upstream Type** should be `service`
(which requires statically listing IP/port combinations for the upstream).

Example:

```
# create a container for the petstore swagger
docker run --name petstore --net docker-compose_default -d soloio/petstore-example:latest

# get its ip
PETSTORE_IP=$(docker inspect petstore -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')

# create the upstream manually
cat <<EOF | glooctl upstream create -f -
type: service
name: petstore
spec:
  hosts:
  - addr: ${PETSTORE_IP}
    port: 8080
EOF

# view functions auto discovered (may take a few seconds)
glooctl upstream get

# create a route
glooctl route create --path-exact /petstore/findPet --upstream petstore --function findPetById

# try the route
curl localhost:8080/petstore/findPet
```

Documentation for [upstream spec](../../docs/v1/upstream.md) and
the [service type](../../docs/plugins/service.md) can explain in more detail
how to create upstreams you need.

When service discovery is supported on Docker this step will no longer be necessary.

Function discovery will still work as normal.
