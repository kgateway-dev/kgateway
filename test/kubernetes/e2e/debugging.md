# Debugging E2e Tests

This document describes workflows that may be useful when debugging e2e tests with an IDE's debugger.

## Overview

The entry point for an e2e test is a Go test function of the form `func TestXyz(t *testing.T)` which represents a top level suite against an installation mode of Gloo. For example, the `TestK8sGateway` function in [k8s_gw_test.go](/test/kubernetes/e2e/k8sgateway/k8s_gw_test.go) is a top-level suite comprising multiple feature specific suites that are invoked as subtests.

Each feature suite is invoked as a subtest of the top level suite. The subtests use [testify](https://github.com/stretchr/testify) to structure the tests in the feature's test suite and make use of the libarary's assertions.

## Workflows

### Running a single feature's suite

Since each feature suite is a subtest of the top level suite, you can run a single feature suite by running the top level suite with the `-run` flag.

For example, to run the `Deployer` feature suite in `TestK8sGateway`, you can run:
```bash
go test -v -timeout 600s ./test/kubernetes/e2e/k8sgateway -run TestK8sGateway/Deployer
```

#### VSCode

In VSCode, this is easily accomplished by invoking the `run test` or `debug test` options when you hover the cursor over the corresponding subtest specified using `t.Run(...)`.

Alternatively, you can use a custom debugger launch config that sets the `test.run` flag to run a specific test:
```
{
  "name": "e2e",
  "type": "go",
  "request": "launch",
  "mode": "test",
  "program": "${workspaceFolder}/test/kubernetes/e2e/k8sgateway/k8s_gw_test.go",
  "args": [
    "-test.run",
    "TestK8sGateway/Deployer",
    "-test.v",
  ],
  "env": {
    "SKIP_INSTALL": "true",
  },
}
```

Setting `SKIP_INSTALL` to `true` will skip the installation of Gloo, which is useful to debug against a pre-existing/stable environment with Gloo already installed.

When invoking tests using VSCode's `run test` option, remember to set `"go.testTimeout": "600s"` in the user `settings.json` file as this may default to a lower value such as `30s` which may not be enough time for the e2e test to complete.

### Running a single test within a feature's suite

Similar to running a specific feature suite, you can run a single test within a feature suite by selecting the test to run using the `-run` flag.

For example, to run `TestProvisionDeploymentAndService` in `Deployer` feature suite that is a part of `TestK8sGateway`, you can run:
```bash
go test -v -timeout 600s ./test/kubernetes/e2e/k8sgateway -run TestK8sGateway/Deployer/TestProvisionDeploymentAndService
```

Alternatively, with VSCode you can use a custom debugger launch config that sets the `test.run` flag to run a specific test:
```
{
  "name": "e2e",
  "type": "go",
  "request": "launch",
  "mode": "test",
  "program": "${workspaceFolder}/test/kubernetes/e2e/k8sgateway/k8s_gw_test.go",
  "args": [
    "-test.run",
    "TestK8sGateway/Deployer/TestProvisionDeploymentAndService",
    "-test.v",
  ],
  "env": {
    "SKIP_INSTALL": "true",
  },
}
```