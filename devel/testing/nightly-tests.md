# Nightly Tests

The following are run on a schedule via a [GitHub action](/.github/workflows/nightly-tests.yaml).

## Gateway API conformance tests
Kubernetes Gateway API conformance tests are run using the earliest and latest supported k8s versions.

## Gateway Load Tests
Kubernetes Gateway load tests are run using the earliest and latest supported k8s versions.

## E2E tests with different Gateway API versions
The entire e2e suite is run against a variety of Gateway API Versions and Channels.