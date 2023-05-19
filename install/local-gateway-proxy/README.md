# Local Gateway-Proxy
It can be useful to run the Envoy proxy, without the control-plane, as a way of validating proxy behavior.

## Setup
[source_data/bootstrap.yaml](./source_data/bootstrap.yaml) provides example bootstrap that can be used. To run this locally, first execute:
```shell
sh prepare-bootstrap.sh
```
to copy the default bootstrap configuration into a new file. This new location (`data/bootstrap.yaml`) is intentionally included in the `.gitigore` so that you can make local edits and not check them into the repository.

## Run
You can either run the gateway-proxy container locally using the default version:
```shell
docker-compose up
```

Or build a local version, and then run that:
```shell
VERSION=<version name> make gloo-envoy-wrapper-docker -B
GLOO_VERSION=<version name> docker-compose up
```

## Debug
Envoy exposes an [administration interface](https://www.envoyproxy.io/docs/envoy/latest/operations/admin) which can be used to query and modify different aspects of the server. The address of this interface is defined in the [bootstrap API](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/bootstrap/v3/bootstrap.proto#envoy-v3-api-msg-config-bootstrap-v3-admin), though it is commonly found at port `19000`.
If the above command succeeded, you should be able to visit [port 19000 in your browser](http://localhost:19000/) to explore the admin interface.

## Cleanup
To clean up the running instance, run:
```shell
docker-compose down
```