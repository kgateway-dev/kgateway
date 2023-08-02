# In Memory End-to-End tests
See the [developer e2e testing guide](/devel/testing/e2e-tests.md) for more information about the philosophy of these tests.

*Note: All commands should be run from the root directory of the Gloo repository*

- [Local Development](#local-development)
    - [Setup](#setup)
        - [Use the CI Install Script](#use-the-ci-install-script)
        - [Verify Your Setup](#verify-your-setup)
        - [Common Setup Errors](#common-setup-errors)
    - [Run Tests](#run-tests)
        - [Using Recently Published Image (Most Common)](#using-recently-published-image-most-common)
        - [Using Previously Published Image](#using-previously-published-image)
        - [Using Locally Built Image](#using-locally-built-image)
        - [Running Tests in Parallel](#running-tests-in-parallel)
    - [Debugging Tests](#debugging-tests)
        - [Use WAIT_ON_FAIL](#use-wait_on_fail)
        - [Use INVALID_TEST_REQS](#use-invalid_test_reqs)
        - 
## Local Development
### Setup
For these tests to run, we require that our gateway-proxy component be previously built as a docker image.

If you have not made local changes to the component, you can rely on a previously published image and no setup is required.

However, if you have made changes to the component, refer to the [Envoyinit README](https://github.com/solo-io/gloo/blob/main/projects/envoyinit) for build instructions.

### Run Tests
The `test` make target runs ginkgo with a set of useful flags. See [run-tests](/devel/testing/run-tests.md) for more details about common techniques for running tests.  The following environment variables can be configured for this target:

| Name              | Default | Description                                                                                                                                                                                                                                        |
|-------------------|---------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| ENVOY_IMAGE_TAG   | ""      | The tag of the gloo-envoy-wrapper-docker image built during setup                                                                                                                                                                                  |
| SERVICE_LOG_LEVEL | ""      | The log levels used for services. See "Controlling Log Verbosity of Services" below.                                                                                                                                                               |    

#### Controlling Log Verbosity of Services
Multiple services (Gloo, Envoy, Discovery) are executed in parallel to run these tests. By default, these services log at the `info` level. To change the log level of a service, set the `SERVICE_LOG_LEVEL` environment variable to a comma separated list of `service:level` pairs.

Options for services are:
- gateway-proxy
- gloo
- uds
- fds

Options for log levels are:
- debug
- info
- warn
- error

For example, to set the log level of the Gloo service to `debug` and the Envoy service to `error`, you would set:

```bash
SERVICE_LOG_LEVEL=gloo:debug,gateway-proxy:error TEST_PKG=./test/e2e/... make test
```

*If the same service has multiple log levels specified, we will log a warning and the last one defined will be used.*

#### Controlling Log Verbosity of Ginkgo Runner
Ginkgo has 4 verbosity settings, whose details can be found in the [Ginkgo docs](https://onsi.github.io/ginkgo/#controlling-verbosity)

To control these settings, you must pass the flags using the `GINKGO_USER_FLAGS` environment variable.

For example, to set the Ginkgo runner to `very verbose` mode, you would set:
```bash
GINKGO_USER_FLAGS=-vv TEST_PKG=./test/e2e/... make test
```

#### Using Recently Published Image (Most Common)
This is the most common pattern. If you did not make changes to the `gateway-proxy` component, and do not specify an `ENVOY_IMAGE_TAG` our tests will identify the most recently published image (for your LTS branch) and use that version.

```bash
TEST_PKG=./test/e2e/... make test
```

#### Using Previously Published Image
If you want to specify a particular version that was previously published, you can also do that by specifying the `ENVOY_IMAGE_TAG`.

```bash
ENVOY_IMAGE_TAG=1.13.0 TEST_PKG=./test/e2e/... make test
```

#### Using Locally Built Image
If you have made changes to the component, you will have had to rebuild the image locally (see [setup tests](#setup)). After you rebuild the image, you need to supply the tag of that image when running the tests:

```bash
ENVOY_IMAGE_TAG=0.0.1-local TEST_PKG=./test/e2e/... make test
```

#### Running Tests in Parallel
It is possible to run tests in parallel locally, however it is not recommended, because debugging failures becomes more difficult. If you do want to run tests in parallel, you can do so by passing in the relevant `GINKGO_USER_FLAGS` values:
```bash
GINKGO_USER_FLAGS=-procs=4 TEST_PKG=./test/e2e/... make test
```

*Note: When using Docker to run Envoy, we have seen occasional failures: `Error response from daemon: dial unix docker.raw.sock: connect: connection refused`*


### Debugging Tests
See [debugging tests](/devel/testing/run-tests.md) for more details about common techniques for debugging tests.
#### Use WAIT_ON_FAIL
When Ginkgo encounters a [test failure](https://onsi.github.io/ginkgo/#mental-model-how-ginkgo-handles-failure) it will attempt to clean up relevant resources, which includes stopping the running instance of Envoy, preventing the developer from inspecting the state of the Envoy instance for useful clues.

To avoid this clean up, run the test(s) with `WAIT_ON_FAIL=1`. When the test fails, it will halt execution, allowing you to inspect the state of the Envoy instance.

Once halted, use `docker ps` to determine the admin port for the Envoy instance, and follow the recommendations for [debugging Envoy](https://github.com/solo-io/gloo/tree/main/projects/envoyinit#debug), specifically the parts around interacting with the Administration interface.

#### Use INVALID_TEST_REQS
Certain test require environmental conditions to be true for them to succeed. For example, certain tests will only run on a Linux machine.

By setting `INVALID_TEST_REQS=skip`, you can run all tests locally, and any tests which will not run in your local environment will be skipped. The default behavior is that they fail.