# Run Tests
- [Background](#background)
- [Common Make Targets](#common-make-targets)
  - [test](#test)
  - [test-with-coverage](#test-with-coverage)
  - [go-test](#go-test)
  - [go-test-with-coverage](#go-test-with-coverage)
- [Environment Variables](#environment-variables)
  - [GINKGO_USER_FLAGS](#ginkgo_user_flags)
  - [TEST_PKG](#test_pkg)
  - [GO_TEST_USER_ARGS](#go_test_user_args)

## Background
Kgateway testing leverages the [Ginkgo](https://onsi.github.io/ginkgo/) test framework. As outlined in the linked documentation, Ginkgo pairs with the [Gomega](https://onsi.github.io/gomega/) matcher library to provide a BDD-style testing framework. For more details about how to write tests, check out our [writing tests docs](writing-tests.md).

## Common Make Targets
There are a few common make targets that can be used to run tests

### test
The `test` target provides a wrapper around invoking `ginkgo` with a set of useful flags. This is the base target that is used by all other test targets.

### test-with-coverage
Run tests with coverage reporting using Ginkgo.

### go-test
Run tests using `go test` directly. This is the primary target used by CI for running tests. Use the `TEST_PKG` environment variable to specify which packages to test, and `GO_TEST_USER_ARGS` to pass additional arguments like `-run` regex patterns for test selection.

### go-test-with-coverage
Run tests with coverage reporting using `go test`. This is used by CI for unit tests

## Environment Variables
Shared environment variables that can be used to control the behavior of the tests are defined in [env.go](/test/testutils/env.go). Below are a few that are commonly used:

#### GINKGO_USER_FLAGS
The `GINKGO_USER_FLAGS` environment variable can be used to pass flags to Ginkgo. For example, to run the tests with very verbose output, you can run:
```bash
GINKGO_USER_FLAGS="-vv" make test
```
*For the full set of available Ginkgo flags, check out the [documentation](https://onsi.github.io/ginkgo/#ginkgo-cli-overview)*

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

#### GO_TEST_USER_ARGS
The `GO_TEST_USER_ARGS` environment variable can be used to pass additional arguments to `go test` when using the `go-test` target. For example, to run specific tests matching a regex pattern:
```bash
TEST_PKG=./test/kubernetes/e2e/tests GO_TEST_USER_ARGS="-run ^TestKgateway$" make go-test
```