# Debugging E2e Tests

This document describes workflows that may be useful when debugging e2e tests with an IDE's debugger.

## Overview

The entry point for an e2e test is a Go test function of the form `func TestXyz(t *testing.T)` which represents a top level suite against an installation mode of kgateway. For example, the `TestKgateway` function in [kgateway_test.go](/test/kubernetes/e2e/tests/kgateway_test.go) is a top-level suite comprising multiple feature specific suites that are invoked as subtests.

Each feature suite is invoked as a subtest of the top level suite. The subtests use [testify](https://github.com/stretchr/testify) to structure the tests in the feature's test suite and make use of the library's assertions.

## Step 1: Setting Up A Cluster
### Using a previously released version
It is possible to run these tests against a previously released version of kgateway. This is useful for testing a release candidate, or a nightly build.

There is no setup required for this option, as the test suite will download the helm chart archive and `glooctl` binary from the specified release. You will use the `RELEASED_VERSION` environment variable when running the tests. See the [variable definition](/test/testutils/env.go) for more details.

### Using a locally built version
For these tests to run, we require the following conditions:
- kgateway helm chart archive present in the `_test` folder
- running kind cluster loaded with the images (with correct tags) referenced in the helm chart

#### Option 1: Using setup-kind.sh script

[hack/kind/setup-kind.sh](/hack/kind/setup-kind.sh) gets run in CI to setup the test environment for the above requirements.
The default settings should be sufficient for a working local environment.
However, the setup script accepts a number of environment variables to control the creation of a kind cluster and deployment of kgateway resources.
Please refer to the script itself to see what variables are available if you need customization.
Additionally, when running on apple silicon architectures, uncheck `Use Rosetta for x86_64/amd64 emulation on Apple Silicon` in your docker settings.

Basic Example:
```bash
./hack/kind/setup-kind.sh
```

#### Option 2: Using Tilt for development workflow

Tilt provides an excellent development workflow for e2e testing as it automatically rebuilds and redeploys images when you make code changes. This is particularly useful for iterative debugging.

**Prerequisites:**

- [Tilt](https://tilt.dev/) installed locally
- [ctlptl](https://github.com/tilt-dev/ctlptl) installed for cluster management

**Setup Steps:**

1. Create a kind cluster with a local registry:

```bash
ctlptl create cluster kind --name kind-kind --registry=ctlptl-registry
```

You can see the status of the cluster with:

```bash
kubectl cluster-info --context kind-kind
```

2. Build and load the initial images:

```bash
VERSION=1.0.0-ci1 CLUSTER_NAME=kind make kind-build-and-load
```

3. Start Tilt to enable live reloading:

```bash
tilt up
```

**Benefits of using Tilt:**

- Automatic image rebuilding and redeployment when code changes are detected
- Live updates without needing to restart the entire test environment
- Web UI for monitoring resource status and logs
- Faster iteration cycles during debugging

For more detailed instructions on using Tilt, see [devel/tilt/tilt.md](/devel/tilt/tilt.md).

## Step 2: Running Tests
_To run the regression tests, your kubeconfig file must point to a running Kubernetes cluster:_
```bash
kubectl config current-context
```
_should run `kind-<CLUSTER_NAME>`_

> Note: If you are running tests against a previously released version, you must set RELEASED_VERSION when invoking the tests

### Running a single feature's suite

Since each feature suite is a subtest of the top level suite, you can run a single feature suite by running the top level suite with the `-run` flag.

For example, to run the `Deployer` feature suite in the `TestKgateway` test:

You can either set environment variables inline with the command:

```bash
SKIP_INSTALL=true CLUSTER_NAME=kind INSTALL_NAMESPACE=kgateway-system go test -v -timeout 600s ./test/kubernetes/e2e/tests -run ^TestKgateway$/^Deployer$
```

Or export the environment variables first and then run the test:

```bash
export SKIP_INSTALL=true
export CLUSTER_NAME=kind
export INSTALL_NAMESPACE=kgateway-system
go test -v -timeout 600s ./test/kubernetes/e2e/tests -run ^TestKgateway$/^Deployer$
```

Note that the `-run` flag takes a sequence of regular expressions, and that each part may match a substring of a suite/test name. See https://pkg.go.dev/cmd/go#hdr-Testing_flags for details. To match only exact suite/test names, use the `^` and `$` characters as shown.

**Additional Environment Variables:**
For a complete list of available environment variables that can be used to configure the test behavior, see [test/testutils/env.go](/test/testutils/env.go). This file contains all the environment variable definitions used by the e2e test suite.

#### VSCode
You can use a custom debugger launch config that sets the `test.run` flag to run a specific test:

```json
{
  "name": "e2e",
  "type": "go",
  "request": "launch",
  "mode": "test",
  "program": "${workspaceFolder}/test/kubernetes/e2e/tests/kgateway_test.go",
  "args": [
    "-test.run",
    "^TestKgateway$/^Deployer$",
    "-test.v",
  ],
  "env": {
    "SKIP_INSTALL": "true",
    "CLUSTER_NAME": "kind",
    "INSTALL_NAMESPACE": "kgateway-system"
  },
}
```

Setting `SKIP_INSTALL` to `true` will skip the installation of kgateway, which is useful to
debug against a pre-existing/stable environment with kgateway already installed.

`CLUSTER_NAME` specifies the name of the cluster used for e2e tests (corresponds to the cluster name used when creating the kind cluster).

`INSTALL_NAMESPACE` specifies the namespace in which kgateway is installed (typically `kgateway-system` when using Tilt).

When invoking tests using VSCode's `run test` option, remember to set `"go.testTimeout": "600s"` in the user `settings.json` file as this may default to a lower value such as `30s` which may not be enough time for the e2e test to complete.

### Running a single test within a feature's suite

Similar to running a specific feature suite, you can run a single test within a feature suite by selecting the test to run using the `-run` flag.

For example, to run `TestProvisionDeploymentAndService` in `Deployer` feature suite that is a part of `TestKgateway`, you can run:
```bash
SKIP_INSTALL=true CLUSTER_NAME=kind INSTALL_NAMESPACE=kgateway-system go test -v -timeout 600s ./test/kubernetes/e2e/tests -run ^TestKgateway$/^Deployer$/^TestProvisionDeploymentAndService$
```

Alternatively, with VSCode you can use a custom debugger launch config that sets the `test.run` flag to run a specific test:
```json
{
  "name": "e2e",
  "type": "go",
  "request": "launch",
  "mode": "test",
  "program": "${workspaceFolder}/test/kubernetes/e2e/tests/kgateway_test.go",
  "args": [
    "-test.run",
    "^TestKgateway$/^Deployer$/^TestProvisionDeploymentAndService$",
    "-test.v",
  ],
  "env": {
    "SKIP_INSTALL": "true",
    "CLUSTER_NAME": "kind",
    "INSTALL_NAMESPACE": "kgateway-system"
  },
}
```

#### Goland

In Goland, you can run a single test feature by right-clicking on the test function and selecting `Run 'TestXyz'` or
`Debug 'TestXyz'`.

You will need to set the env variable `SKIP_INSTALL` to `true` in the run configuration to skip the installation of Gloo. This
is also the case for other env variables that are required for the test to run (`CLUSTER_NAME`, etc.)

If there are multiple tests in a feature suite, you can run a single test by adding the test name to the `-run` flag in the run configuration:

```bash
-test.run="^TestKgateway$/^RouteOptions$/^TestConfigureRouteOptionsWithTargetRef$"
```


### Running the same tests as our CI pipeline
We [load balance tests](./load_balancing_tests.md) across different clusters when executing them in CI. If you would like to replicate the exact set of tests that are run for a given cluster, you should:
1. Inspect the `go-test-run-regex` defined in the [test matrix](/.github/workflows/pr-kubernetes-tests.yaml)
```bash
go-test-run-regex: '(^TestKgateway$$)'
```
_NOTE: There is `$$` in the GitHub action definition, since a single `$` is expanded_
2. Inspect the `go-test-args` defined in the [test matrix](/.github/workflows/pr-kubernetes-tests.yaml)
```bash
go-test-args: '-v -timeout=25m'
```
3. Combine these arguments when invoking go test:
```bash
TEST_PKG=./test/kubernetes/e2e/... GO_TEST_USER_ARGS='-v -timeout=25m -run \(^TestKgateway$$/\)' make go-test
```
