# Kubernetes End-to-End tests
See the [developer kube-e2e testing guide](/devel/testing/kube-e2e-tests.md) for more information about the philosophy of these tests.

*Note: All commands should be run from the root directory of the Gloo repository*

- [Local Development](#local-development)
    - [Setup](#setup)
        - [Use the CI Install Script](#use-the-ci-install-script)
        - [Verify Your Setup](#verify-your-setup)
        - [Common Setup Errors](#common-setup-errors)
    - [Run Tests](#run-tests)
        - [Use the Make Target](#use-the-make-target)
        - [Test Environment Variables](#test-environment-variables)
        - [Common Test Errors](#common-test-errors)

## Local Development
### Setup (Previously Released Assets)
It is possible to run these tests against a previously released version of Gloo Edge. This is useful for testing a release candidate, or a nightly build.

There is no setup required for this option, as the test suite will download the helm chart archive and `glooctl` binary from the specified release. You will use the `RELEASED_VERSION` environment variable when running the tests. See the [variable definition](/test/testutils/env.go) for more details.

### Setup (Locally Build Assets)

For these tests to run, we require the following conditions:
- Gloo Edge Helm chart archive is present in the `_test` folder,
- `glooctl` is built in the `_output` folder
- A KinD cluster is set up and loaded with the images to be installed by the helm chart

#### Use the CI Install Script
[ci/kind/setup-kind.sh](/ci/kind/setup-kind.sh) gets run in CI to setup the test environment for the above requirements.
It accepts a number of environment variables, to control the creation of a kind cluster and deployment of Gloo resources to that kind cluster.

| Name                 | Default  | Description                                                                                                         |
|----------------------|----------|---------------------------------------------------------------------------------------------------------------------|
| CLUSTER_NAME         | kind     | The name of the cluster that will be generated                                                                      |
| CLUSTER_NODE_VERSION | v1.28.0  | The version of the [Node Docker image](https://hub.docker.com/r/kindest/node/) to use for booting the cluster       |
| VERSION              | 1.0.0-ci | The version used to tag Gloo images that are deployed to the cluster                                                |
| KUBE2E_TESTS         | gateway  | Name of the test suite to be run. Options: `'gateway', 'gloo', 'ingress', 'helm', 'gloomtls', 'glooctl', 'upgrade'` |
| SKIP_DOCKER          | false    | Skip building docker images (used when testing a release version)                                                   |
| RELEASED_VERSION     | ''       | Used if you want to test a previously released version. 'LATEST' will find the latest release                       |

Example:
```bash
CLUSTER_NAME=solo-test-cluster CLUSTER_NODE_VERSION=v1.28.0 VERSION=v1.0.0-solo-test ci/kind/setup-kind.sh
```

#### Verify Your Setup
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
KUBE2E_TESTS=<test-to-run> make run-kube-e2e-tests
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