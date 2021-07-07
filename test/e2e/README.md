# End-to-end tests
This directory contains end-to-end tests that do not require kubernetes

*Note: All commands should be run from the root directory of the Gloo repository*

## (Option A) Run Envoy in a Docker Container
### Setup
For these tests to run, we require Envoy be built in a docker container. The VERSION env variable determines the name of the tag for that image.

```bash
VERSION=<version-name> make gloo-envoy-wrapper-docker
```

### Run Tests
The `run-tests` make target runs ginkgo with a set of useful flags. The following environment variables can be configured for this target:

| Name            | Default | Description |
| ---             |   ---   |    ---      |
| ENVOY_IMAGE_TAG | ""      | The tag of the gloo-envoy-wrapper-docker image built during setup |
| TEST_PKG        | ""      | The path to the package of the test suite you want to run  |
| WAIT_ON_FAIL    | 0       | Set to 1 to prevent Ginkgo from cleaning up the Gloo Edge installation in case of failure. Useful to exec into inspect resources created by the test. A command to resume the test run (and thus clean up resources) will be logged to the output.

Example:
```bash
ENVOY_IMAGE_TAG=solo-test-image TEST_PKG=./test/e2e/... make run-tests
```

## (Option B) Run Envoy as a Binary
*Note: We need someone to update these instructions*


## Additional Notes

#### Notes on EC2 tests
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

