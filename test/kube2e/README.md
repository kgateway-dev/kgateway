# Regression tests
This directory contains test that install each of the 3 Gloo Edge flavours (`gateway`, `ingress`, and `knative`) and run 
regression tests against them.

## Build test assets
The tests require that a Gloo Edge Helm chart archive be present in the `_test` folder. This chart will be used to install 
Gloo Edge to the GKE `kube2e-tests` cluster (by running `glooctl install <deployment-type> -f -test/<chart-archive-name>`).

To build the chart, execute the `docker` and `build-test-assets` make targets:

```bash
make GCLOUD_PROJECT_ID=solo-public BUILD_ID=my-local-build docker build-test-assets
```

The above command will also build all our docker images and deploy them to Google Container Registry (GCR), where the 
image references in the chart expect them to be.

## Run test
To run the regression tests, your kubeconfig file must point to a running Kubernetes cluster. You can then start the 
tests by running the following command from this directory:

```bash
ginkgo -r
```

Although running tests in parallel *should* work, the fact that Gloo Edge creates some cluster-scoped resources is a 
potential source of problems.

### Test environment variables
The below table contains the environment variables that can be used to configure the test execution.

| Name              | Required  | Description |
| ---               |   ---     |    ---      |
| RUN_KUBE2E_TESTS  | Y         | Must be set to 1, otherwise tests will be skipped |
| DEBUG             | N         | Set to 1 for debug log output |
| WAIT_ON_FAIL      | N         | Set to 1 to prevent Ginkgo from cleaning up the Gloo Edge installation in case of failure. Useful to exec into inspect resources created by the test. A command to resume the test run (and thus clean up resources) will be logged to the output.


### To run locally with kind:

```bash
kind create cluster
VERSION=kind ./ci/kind.sh
GO111MODULE=off go get -u github.com/onsi/ginkgo/ginkgo
make glooctl-darwin-amd64 # if you are on a mac
make glooctl-linux-amd64 # if you are on linux
# To run tests that require a cluster lock
ginkgo -r -failFast -trace -progress -race -compilers=4 -failOnPending -noColor ./test/kube2e/...
```