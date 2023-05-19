# Gloo

## Build
*All make targets are currently defined in the [Makefile](https://github.com/solo-io/gloo/blob/main/Makefile) at the root of the repository.*

The `VERSION` env variable determines the name of the tag for the image.

You may either inject the version yourself:
```bash
VERSION=<version name> make gloo-docker -B
```

Or rely on the auto-generated version:
```shell
make gloo-docker -B
```

## Release
During a Gloo Edge release, the `gloo` image is published to the [Google Cloud Registry](https://console.cloud.google.com/gcr/images/gloo-edge/GLOBAL) and the [Quay repository](https://quay.io/repository/solo-io/gloo).

## Components

### xDS Server
Gloo sends Envoy dynamic configuration via the [xDS protocol](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol#xds-protocol). The Gloo [xDS](https://github.com/solo-io/gloo/tree/main/projects/gloo/pkg/xds) package contains relevant code for serving dynamic configuration.


## Testing
Tests are run using [Ginkgo](https://onsi.github.io/ginkgo/).

`make test` is the entrypoint for running the unit tests in the `TEST_PKG`

To run all tests in this project:
```make
TEST_PKG=projects/gloo make test
```

To run a specific subset of tests, read the Ginkgo docs around [focusing tests](https://onsi.github.io/ginkgo/#focused-specs)
```make
TEST_PKG=projects/gloo/pkg make test
```