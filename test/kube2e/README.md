# Kubernetes End-to-End tests
This directory contains tests that install each of the 3 Gloo Edge flavors (`gateway`, `ingress`, and `knative`) in a Kubernetes cluster, and run
end-to-end tests against them.

*Note: All commands should be run from the root directory of the Gloo repository*

## Background
Kubernetes may be relied on for scheduling, persistence or security. These tests validate that Gloo Edge can successfully operate within a Kubernetes cluster.

### How do the tests work?
1. Install Gloo Edge in Kubernetes cluster [using Helm](https://github.com/solo-io/gloo/blob/1f457f4ef5f32aedabc58ef164aeea92acbf481e/test/kube2e/gateway/gateway_suite_test.go#L84)
1. Apply Gloo resources using Kubernetes resource clients
1. Execute requests against the Envoy proxy and confirm the expected response. This validates that the Gloo resources have been picked up by the controllers, were been translated correctly into Envoy configuration, the configuration was sent to the Envoy proxy, and the proxy behaves appropriately.

## CI
These tests are run by a [GitHub action](https://github.com/solo-io/gloo/blob/main/.github/workflows/regression-tests.yaml) as part of our CI pipeline.

If a test fails, you can retry it from a [browser window](https://docs.github.com/en/actions/managing-workflow-runs/re-running-workflows-and-jobs#reviewing-previous-workflow-runs). If you do this, please make sure to comment on the Pull Request with a link to the failed logs for debugging purposes.

## Nightly runs
Tests are also run on a schedule via another [GitHub action](https://github.com/solo-io/gloo/blob/main/.github/workflows/nightly-tests.yaml). The nightly tests use the latest release - specified with the RELEASED_VERSION environment variable. 
### Extra considerations for running from released builds
The `GetTestHelper` util method handles installing gloo from either a local or released build. When testing released builds, tests that interact directly with the helm chart need to download the chart using the version stored in `testHelper.GetChartVersion()`

## Adding new tests
The list of tests to run during CI and nightly builds is provided in `kube-e2e-test-type` matrices in the github workflows. A new test can be added to one or both lists of tests.  
## Local Development

### Setup
For these tests to run, we require the following conditions:
  - Gloo Edge Helm chart archive be present in the `_test` folder,
  - `glooctl` be built in the`_output` folder
  - kind cluster set up and loaded with the images to be installed by the helm chart

#### Use the CI Install Script
[ci/deploy-to-kind-cluster.sh](`https://github.com/solo-io/gloo/blob/main/ci/deploy-to-kind-cluster.sh`) gets run in CI to setup the test environment for the above requirements.
It accepts a number of environment variables, to control the creation of a kind cluster and deployment of Gloo resources to that kind cluster.

| Name                 | Default  | Description                                                                                                         |
|----------------------|----------|---------------------------------------------------------------------------------------------------------------------|
| CLUSTER_NAME         | kind     | The name of the cluster that will be generated                                                                      |
| CLUSTER_NODE_VERSION | v1.25.3  | The version of the [Node Docker image](https://hub.docker.com/r/kindest/node/) to use for booting the cluster       |
| VERSION              | 1.0.0-ci | The version used to tag Gloo images that are deployed to the cluster                                                |
| KUBE2E_TESTS         | gateway  | Name of the test suite to be run. Options: `'gateway', 'gloo', 'ingress', 'helm', 'gloomtls', 'glooctl', 'upgrade'` |
| SKIP_DOCKER          | false    | Skip building docker images (used when testing a release version)                                                   |

Example:
```bash
CLUSTER_NAME=solo-test-cluster CLUSTER_NODE_VERSION=v1.25.3 VERSION=v1.0.0-solo-test ci/deploy-to-kind-cluster.sh
```

### Verify Your Setup
Before running your tests, it's worthwhile to verify that a cluster was created, and the proper images have been loaded.

```bash
CLUSTER_NAME=solo-test-cluster make kind-list-images
```
You should see the list of images in the cluster, including the ones you just uploaded

#### Common Setup Errors
`Error: validation: chart.metadata.version "solo" is invalid`\
In newer versions of helm (>3.5), the version used to build the helm chart (ie the VERSION env variable), needs to respect semantic versioning. This error implies that the version provided does not.

Prepend a valid semver to avoid the error. (ie `kind` can become `0.0.0-kind1`)

### Run Tests
To run the regression tests, your kubeconfig file must point to a running Kubernetes cluster.
`kubectl config current-context` should run `kind-<CLUSTER_NAME>`

#### Use the Make Target

Use the same command that CI relies on:
```bash
KUBE2E_TESTS=<test-to-run> make run-ci-regression-tests
```

#### Test Environment Variables
The below table contains the environment variables that can be used to configure the test execution.

| Name             | Default | Description                                                                                                                                                                                                                                        |
|------------------|---------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| KUBE2E_TESTS     | gateway | Name of the test suite to be run. Options: `'gateway', 'gloo', 'ingress', 'helm', 'gloomtls', 'glooctl', 'upgrade'`                                                                                                                                |
| DEBUG            | 0       | Set to 1 for debug log output                                                                                                                                                                                                                      |
| WAIT_ON_FAIL     | 0       | Set to 1 to prevent Ginkgo from cleaning up the Gloo Edge installation in case of failure. Useful to exec into inspect resources created by the test. A command to resume the test run (and thus clean up resources) will be logged to the output. |
| TEAR_DOWN        | false   | Set to true to uninstall Gloo after the test suite completes                                                                                                                                                                                       |
| RELEASED_VERSION | ''      | Used by nightlies to tests a specific released version. 'LATEST' will find the latest release                                                                                                                                                      |
#### Common Test Errors
`getting Helm chart version: expected a single entry with name [gloo], found: 5`\
The test helm charts are written to the `_test` directory, with the `index.yaml` file containing references to all available charts. The tests require that this file contain only 1 entry. Delete the other entries manually, or run `make clean` to delete this folder entirely, and then re-build the test helm chart.