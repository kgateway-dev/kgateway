# Nightly Tests



## Nightly runs
Tests are also run on a schedule via another [GitHub action](/.github/workflows/nightly-tests.yaml). The nightly tests use the latest release - specified with the RELEASED_VERSION environment variable.
### Extra considerations for running from released builds
The `GetTestHelper` util method handles installing gloo from either a local or released build. When testing released builds, tests that interact directly with the helm chart need to download the chart using the version stored in `testHelper.GetChartVersion()`

