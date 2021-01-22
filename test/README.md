## Setup for running Gloo tests locally 

### Consult Vault Test Setup 

The consul vault test downloads and runs vault and is disabled by default. To enable, set `RUN_VAULT_TESTS=1` and `RUN_CONSUL_TESTS=1` in your local environment.

### e2e Test Setup

Run `TAGGED_VERSION=v${NAME} make gloo-envoy-wrapper-docker`, then set the `ENVOY_GLOO_IMAGE` to the `TAGGED_VERSION` name.

### Kube e2e Test Setup

Instructions for setting up and running the regression tests can be found [here](https://github.com/solo-io/gloo/tree/master/test/kube2e#regression-tests).