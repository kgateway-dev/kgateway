# Envoy Dynamic Module

This module initially was roughly Based on https://github.com/envoyproxy/dynamic-modules-examples

Currently, the module only has an experimental transformation implementation named "rustformations" that will eventually replace the C++ native transformation from envoy-gloo.

## Project Organization

This module is organized as [Rust Workspaces](https://doc.rust-lang.org/cargo/reference/workspaces.html) with the following crates:

- rustformations: This crate is the skeleton code that setup all the hooks for the envoy dynamic module. All envoy dependencies are contained in the crate.
- transformations: This is where the actual transformation logic is implemented which includes the inja template dependencies.

## Building

The Dockerfile that build the envoy wrapper image is in /cmd/envoyinit/Dockerfile. It will pull in the envoy binary, this dynamic module and the envoyinit binary into the image.
To build the envoy wrapper docker image, at the kgateway top project level, do:

``` bash
make envoy-wrapper-docker
```

A custom ENVOY_IMAGE can be used but make sure the rust sdk is compatible with the rustformation module:

``` bash
ENVOY_IMAGE=<custom envoy image> make envoy-wrapper-docker
```

## Formatting and Linting

Before creating a PR, run:

``` bash
make lint
```

## Testing

### unit testing

To run unit tests, do:

``` bash
cargo test
```

### e2e testing

At the kgateway project top level directory, run:

``` bash
hack/run-e2e-test.sh TestKgateway/^Transforms$/TestGatewayWithTransformation
```

## Envoy upgrade

The envoy sha in the Cargo dependencies need to match the envoy version being used. See [envoy-upgrade](../../devel/envoy/envoy-upgrade.md) for details.
