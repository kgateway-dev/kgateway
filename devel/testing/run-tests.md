# Run Tests

## Background


## Common Make Targets
### test


### test-with-coverage

### run-tests

### run-e2e-tests

### run-kube-e2e-tests

### run-benchmark-tests

There are a few common make targets that can be used to run tests:

## Environment Variables
#### GINKGO_USER_FLAGS
The `GINKGO_USER_FLAGS` environment variable can be used to pass flags to Ginkgo. For example, to run the tests with very verbose output, you can run:
```bash
GINKGO_USER_FLAGS="-vv" make test
````

For the full set of available Ginkgo flags, check out the [documentation](https://onsi.github.io/ginkgo/#ginkgo-cli-overview)

#### TEST_PKG
The `TEST_PKG` environment variable can be used to run a specific test suite. For example, to run the `test` test suite, you can run:
```bash
TEST_PKG=test make test
```

If you would like to run multiple test suites, you can separate them with a comma:
```bash
TEST_PKG=package1,package2 make test
```

If you would like to recursively run tests in a directory, you can use the `...` syntax:
```bash
TEST_PKG=test/... make test
```

#### WAIT_ON_FAIL
The `WAIT_ON_FAIL` environment variable can be used to prevent Ginkgo from cleaning up the Gloo Edge installation in case of failure. This is useful to inspect resources created by the test. A command to resume the test run (and thus clean up resources) will be logged to the output.

See [the definition](/test/testutils/env.go) for more details about when it can be used.

#### INVALID_TEST_REQS
The `INVALID_TEST_REQS` environment variable can be used to control the behavior of tests which depend on environment conditions that aren't satisfied. Options are `skip`, `run`, `fail`. The default is `fail`.

For example, to skip tests that contain requirements which your machine doesn't support, run:
```bash
INVALID_TEST_REQS=skip make test
```