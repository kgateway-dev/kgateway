# Consul/Vault Tests

## Setup
The consul vault test downloads and runs vault and is disabled by default. To enable, set `RUN_VAULT_TESTS=1` and `RUN_CONSUL_TESTS=1` in your local environment.


# Consul/Vault End-to-End tests
This directory contains end-to-end tests that do not require Kubernetes

*Note: All commands should be run from the root directory of the Gloo repository*

## CI
These tests are run by [build-bot](https://github.com/solo-io/build-bot) in Google Cloud as part of our CI pipeline.

If a test fails, you can retry it using the build-bot [comment directives](https://github.com/solo-io/build-bot#issue-comment-directives). If you do this, please make sure to include a link to the failed logs for debugging purposes.

## Local Development

### Setup
For these tests to run, we require Envoy be built in a docker container.

Refer to the [Envoyinit README](https://github.com/solo-io/gloo/blob/master/projects/envoyinit) for build instructions.

These tests are disabled by default. To enable, set `RUN_VAULT_TESTS=1` and `RUN_CONSUL_TESTS=1` in your local environment.

### Run Tests
The `run-tests` make target runs ginkgo with a set of useful flags. The following environment variables can be configured for this target:

| Name            | Default | Description |
| ---             |   ---   |    ---      |
| RUN_VAULT_TESTS | 0       | Set to 1 to enable Vault tests (required for this suite) |
| RUN_CONSUL_TESTS | 0       | Set to 1 to enable Consul tests (required for this suite) |
| ENVOY_IMAGE_TAG | ""      | The tag of the gloo-envoy-wrapper-docker image built during setup |
| TEST_PKG        | ""      | The path to the package of the test suite you want to run  |
| WAIT_ON_FAIL    | 0       | Set to 1 to prevent Ginkgo from cleaning up the Gloo Edge installation in case of failure. Useful to exec into inspect resources created by the test. A command to resume the test run (and thus clean up resources) will be logged to the output.

Example:
```bash
RUN_CONSUL_TESTS=1 RUN_VAULT_TESTS=1 TEST_PKG=./test/consulvaulte2e/... ENVOY_IMAGE_TAG=solo-test-image WAIT_ON_FAIL=1 make run-tests
```


### Debugging Tests

#### Use WAIT_ON_FAIL
When Ginkgo encounters a [test failure](https://onsi.github.io/ginkgo/#mental-model-how-ginkgo-handles-failure) it will attempt to clean up relevant resources, which includes stopping the running instance of Envoy, preventing the developer from inspecting the state of the Envoy instance for useful clues.

To avoid this clean up, run the test(s) with `WAIT_ON_FAIL=1`. When the test fails, it will halt execution, allowing you to inspect the state of the Envoy instance.

Once halted, use `docker ps` to determine the admin port for the Envoy instance, and follow the recommendations for [debugging Envoy](https://github.com/solo-io/gloo/tree/master/projects/envoyinit#debug), specifically the parts around interacting with the Administration interface.