# End-to-End Testing Framework

## Testify

We rely on [testify](https://github.com/stretchr/testify) to provide the structure for our end-to-end testing. This allows us to decouple where tests are defined, from where they are run.

## TestCluster

A [TestCluster](./test.go) is the structure that manages tests running against a single Kubernetes Cluster.

Its sole responsibility is to create [TestInstallations](#testinstallation).

## TestInstallation

A [TestInstallation](./test.go) is the structure that manages a group of tests that run against an installation within a Kubernetes Cluster.

We try to define a single `TestInstallation` per file in a `TestCluster`. This way, it is easy to identify what behaviors are expected for that installation.

## Features

We define all tests in the [features](./features) package. This is done for a variety of reasons:

1. We group the tests by feature, so it's easy to identify which behaviors we assert for a given feature.
2. We can invoke that same test against different `TestInstallation`s. This means we can test a feature against a variety of installation values, or even against OSS and Enterprise installations.

Many examples of testing features may be found in the [features](./features) package. The general pattern for adding a new feature should be to create a directory for the feature under `features/`, write manifest files for the resources the tests will need into `features/my_feature/testdata/`, define Go objects for them in a file called `features/my_feature/types.go`, and finally define the test suite in `features/my_feature/suite.go`. There are occasions where multiple suites will need to be created under a single feature. See [Suites](#test-suites) for more info on this case.

## Test Suites

A Test Suite is a subset of the Feature concept. A single Feature has at minimum one Test Suite, and can have many. Each Test Suite should have its own appropriately named `.go` file from which is exported an appropriately named function which satisfies the signature `NewSuiteFunc` found in [suite.go](./suite.go).

These test suites are registered by a name and this func in [Tests](#tests) to be run against various `TestInstallation`s.

## Tests

This package holds the entry point for each of our `TestInstallation`.

See [Load balancing tests](./load_balancing_tests.md) for more information about how these tests are run in CI.

Each `*_test.go` file contains a specific test installation and exists within the `tests_test` package. In order for tests to be imported and run from other repos, each `*_test.go` file has a corresponding `*_test.go` file which exists in the `tests` package. This is done because `_test` packages cannot be imported.

In order to add a feature suite to be run in a given test installation, it must be added to the exported function in the corresponding `*_tests.go` file.
e.g. In order to add a feature suite to be run with the test installation defined in `istio_test.go`, we have to register it by adding it to `IstioTests()` in `istio_tests.go` following the existing paradigm.

## Adding Tests to CI

When writing new tests, they should be added to the the [`Kubernetes Tests` that run on all PRs](/.github/workflows/pr-kubernetes-tests.yaml) if they are not already covered by an existing regex. This way we ensure parity between PR runs and nightlies.

When adding it to the list, ensure that the tests are load balanced to allow quick iteration on PRs and update the date and the duration of corresponding test.
The only exception to this is the Upgrade tests that are not run on the main branch but all LTS branches.

## Environment Variables

Some tests may require environment variables to be set. Some required env vars are:

- Istio features: Require `ISTIO_VERSION` to be set. The tests running in CI use `ISTIO_VERSION="${ISTIO_VERSION:-1.19.9}"` to default to a specific version of Istio.

## Resource example generation tool

This tool generates the input resources defined in code as an output yaml file. You can find an example under `test/kubernetes/e2e/features/headless_svc/generate/generate_examples.go`.

These examples are run as part of the codegen, but can also be manually run using the following command:

```bash
go generate <path to the generate.go file>
```

## Debugging

Refer to the [Debugging guide](./debugging.md) for more information on how to debug tests.

## Thanks

### Inspiration

This framework was inspired by the following projects:

- [Kubernetes Gateway API](https://github.com/kubernetes-sigs/gateway-api/tree/main/conformance)

### Areas of Improvement
>
> **Help Wanted:**
> This framework is not feature complete, and we welcome any improvements to it.

Below are a set of known areas of improvement. The goal is to provide a starting point for developers looking to contribute. There are likely other improvements that are not currently captured, so please add/remove entries to this list as you see fit:

- **Debug Improvements**: On test failure, we should emit a report about the entire state of the cluster. This should be a CLI utility as well.
- **Curl assertion**: We need a re-usable way to execute Curl requests against a Pod, and assert properties of the response.
- **Cluster provisioning**: We rely on the [setup-kind](/hack/kind/setup-kind.sh) script to provision a cluster. We should make this more flexible by providing a configurable, declarative way to do this.
- **Istio action**: We need a way to perform Istio actions against a cluster.
- **Argo action**: We need an easy utility to perform ArgoCD commands against a cluster.
