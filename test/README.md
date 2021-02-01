## Setup for running Gloo tests locally 

### Consult Vault Test Setup 

The consul vault test downloads and runs vault and is disabled by default. To enable, set `RUN_VAULT_TESTS=1` and `RUN_CONSUL_TESTS=1` in your local environment.

### e2e Test Setup

If running the e2e tests on a Mac, you will need to run `TAGGED_VERSION=v${NAME} make gloo-envoy-wrapper-docker`, then set the `ENVOY_GLOO_IMAGE` to the `TAGGED_VERSION` name. You can run the tests using a binary on linux.

### Kube e2e Test Setup

Instructions for setting up and running the regression tests can be found [here](https://github.com/solo-io/gloo/tree/master/test/kube2e#regression-tests).

## Debugging Tests

# Gloo Tests

Some of the gloo tests use a listener on 127.0.0.1 rather than 0.0.0.0 and will only run on linux (e.g. fault injection).

If you’re developing on a mac, ValidateBootstrap will not run properly because it uses the envoy binary for validation mode (which only runs on linux). See rbac_jwt_test.go for an example.