# Nightly Tests

The following are run on a schedule via a [GitHub action](/.github/workflows/nightly-tests.yaml).

## Kubernetes End-to-End Tests
Our [kube-e2e-tests](kube-e2e-tests.md) are run using the earlier and latest supported k8s versions. These tests use the latest release - specified with the `RELEASED_VERSION` environment variable.

This is in addition to the running of kubernetes end-to-end tests as part of CI.


## Performance Tests
See [performance-tests](performance-tests.md) for more details about performance tests
