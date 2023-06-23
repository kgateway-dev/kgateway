# In Memory End-to-End tests
This directory contains end-to-end tests that do not require Kubernetes, and persist configuration in memory.

*Note: All commands should be run from the root directory of the Gloo repository*

## Background
This is the most common type of end-to-end test, since it is the quickest to set up and easiest to debug. Additionally, since Gloo Edge may be run using various backing stores, these tests provide a single space to validate the translation of Gloo resources into Envoy resources, independent of where Gloo Edge is deployed. As a result, these test do not rely on Kubernetes, so if there is any Kubernetes behavior that needs to be tested, write a [kubernetes end-to-end test](../kube2e) instead.

### How do the tests work?
1. Run the [Gloo controllers in goroutines](https://github.com/solo-io/gloo/blob/1f457f4ef5f32aedabc58ef164aeea92acbf481e/test/services/gateway.go#L109)
1. Run [Envoy](https://github.com/solo-io/gloo/blob/1f457f4ef5f32aedabc58ef164aeea92acbf481e/test/services/envoy.go#L237) either using a binary or docker container
1. Apply Gloo resources using [in-memory resource clients](https://github.com/solo-io/gloo/blob/1f457f4ef5f32aedabc58ef164aeea92acbf481e/test/services/gateway.go#L175)
1. Execute requests against the Envoy proxy and confirm the expected response. This validates that the Gloo resources have been picked up by the controllers, were been translated correctly into Envoy configuration, the configuration was sent to the Envoy proxy, and the proxy behaves appropriately.

### Example Test
We have an [example test](./example_test.go) which outlines how these tests work. It also provides some examples for basic testing operations. If you are writing a new e2e test, we recommend that you review the example test first.

This was introduced in a [pull request](https://github.com/solo-io/gloo/pull/7555) which includes other useful details about e2e test considerations.

## CI
These tests are run by [build-bot](https://github.com/solo-io/build-bot) in Google Cloud as part of our CI pipeline.

If a test fails, you can retry it using the build-bot [comment directives](https://github.com/solo-io/build-bot#issue-comment-directives). If you do this, please make sure to include a link to the failed logs for debugging purposes.

## Local Development
### Setup
For these tests to run, we require that our gateway-proxy component be previously built as a docker image.

If you have not made local changes to the component, you can rely on a previously published image and no setup is required.

However, if you have made changes to the component, refer to the [Envoyinit README](https://github.com/solo-io/gloo/blob/main/projects/envoyinit) for build instructions.

### Run Tests
The `run-tests` make target runs ginkgo with a set of useful flags. The following environment variables can be configured for this target:

| Name              | Default | Description                                                                                                                                                                                                                                        |
|-------------------|---------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| ENVOY_IMAGE_TAG   | ""      | The tag of the gloo-envoy-wrapper-docker image built during setup                                                                                                                                                                                  |
| TEST_PKG          | ""      | The path to the package of the test suite you want to run                                                                                                                                                                                          |
| WAIT_ON_FAIL      | 0       | Set to 1 to prevent Ginkgo from cleaning up the Gloo Edge installation in case of failure. Useful to exec into inspect resources created by the test. A command to resume the test run (and thus clean up resources) will be logged to the output. |
| INVALID_TEST_REQS | fail    | The behavior for tests which depend on environment conditions that aren't satisfied. Options are `skip`, `run`, `fail`                                                                                                                             |
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

### Debugging Tests
#### Use WAIT_ON_FAIL
When Ginkgo encounters a [test failure](https://onsi.github.io/ginkgo/#mental-model-how-ginkgo-handles-failure) it will attempt to clean up relevant resources, which includes stopping the running instance of Envoy, preventing the developer from inspecting the state of the Envoy instance for useful clues.

To avoid this clean up, run the test(s) with `WAIT_ON_FAIL=1`. When the test fails, it will halt execution, allowing you to inspect the state of the Envoy instance.

Once halted, use `docker ps` to determine the admin port for the Envoy instance, and follow the recommendations for [debugging Envoy](https://github.com/solo-io/gloo/tree/main/projects/envoyinit#debug), specifically the parts around interacting with the Administration interface.

#### Use INVALID_TEST_REQS
Certain test require environmental conditions to be true for them to succeed. For example, certain tests will only run on a Linux machine.

By setting `INVALID_TEST_REQS=skip`, you can run all tests locally, and any tests which will not run in your local environment will be skipped. The default behavior is that they fail.

## Additional Notes
### Notes on EC2 tests
*Note: these instructions are out of date, and require updating*

- set up your ec2 instance
  - download a simple echo app
  - make the app executable
  - run it in the background

```bash
wget https://mitch-solo-public.s3.amazonaws.com/echoapp2
chmod +x echoapp2
sudo ./echoapp2 --port 80 &
```

### Notes on AWS Lambda Tests (`test/e2e/aws_test.go`)

In addition to the configuration steps provided above, you will need to do the following to run the [AWS Lambda Tests](https://github.com/solo-io/gloo/blob/main/test/e2e/aws_test.go) locally:
  1. Obtain an AWS IAM User account that is part of the Solo.io organization
  2. Create an AWS access key
       - Sign into the AWS console with the account created during step 1
       - Hover over your username at the top right of the page. Click on "My Security Credentials"
       - In the section titled "AWS IAM credentials", click "Create access key" to create an acess key
       - Save the Access key ID and the associated secret key
  3. Install AWS CLI v2
       - You can check whether you have AWS CLI installed by running `aws --version`
       - Installation guides for various operating systems can be found [here](https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html)
  4. Configure AWS credentials on your machine
       - You can do this by running `aws configure`
       - You will be asked to provide your Access Key ID and Secret Key from step 2, as well as the default region name and default output format
         - It is critical that you set the default region to `us-east-1`
       - This will create a credentials file at `~/.aws/credentials` on Linux or macOS, or at `C:\Users\USERNAME\.aws\credentials` on Windows. The tests read this file in order to interact with lambdas that have been created in the Solo.io organization
    