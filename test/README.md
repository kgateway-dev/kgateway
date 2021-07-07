## Setup for running Gloo tests locally

### e2e Tests

Instructions for setting up and running the end-to-end tests can be found [here](https://github.com/solo-io/gloo/tree/master/test/e2e#end-to-end-tests).

### Kube e2e Tests

Instructions for setting up and running the regression tests can be found [here](https://github.com/solo-io/gloo/tree/master/test/kube2e#regression-tests).

### Consult Vault Test Setup

The consul vault test downloads and runs vault and is disabled by default. To enable, set `RUN_VAULT_TESTS=1` and `RUN_CONSUL_TESTS=1` in your local environment.

## Debugging Tests

# Gloo Tests

Some of the gloo tests use a listener on 127.0.0.1 rather than 0.0.0.0 and will only run on linux (e.g. fault injection).

If you’re developing on a mac, ValidateBootstrap will not run properly because it uses the envoy binary for validation mode (which only runs on linux). See rbac_jwt_test.go for an example.