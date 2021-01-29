# Regression tests
This directory contains tests that install each of the 3 Gloo Edge flavors (`gateway`, `ingress`, and `knative`) and run
regression tests against them.

## Setup
These tests require that a Gloo Edge Helm chart archive be present in the `_test` folder, `glooctl` be built in the
`_output` folder, and a kind cluster set up and loaded with the images to be installed by the helm chart.

`ci/kind.sh` gets run in ci to setup the test environment for the above requirements. To run tests locally, it is
recommended you create a kind cluster using the same command there, build a helm chart for the version you
want to test (`VERSION=kind make build-test-chart`), build glooctl (`make glooctl`) and load the docker images into
kind that will get installed (`CLUSTER_NAME=kind VERSION=kind make push-kind-images`).

## Run test
To run the regression tests, your kubeconfig file must point to a running Kubernetes cluster. You can then start the 
tests by running the following command from this directory:

```bash
KUBE2E_TESTS=<test-to-run> ginkgo -r
```

### Test environment variables
The below table contains the environment variables that can be used to configure the test execution.

| Name              | Required  | Description |
| ---               |   ---     |    ---      |
| KUBE2E_TESTS      | Y         | Must be set to the test suite to be run, otherwise all tests will be skipped |
| DEBUG             | N         | Set to 1 for debug log output |
| WAIT_ON_FAIL      | N         | Set to 1 to prevent Ginkgo from cleaning up the Gloo Edge installation in case of failure. Useful to exec into inspect resources created by the test. A command to resume the test run (and thus clean up resources) will be logged to the output.
