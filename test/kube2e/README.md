# Regression tests

This directory contains regression tests for the 3 versions of


## Build test assets

The tests require that a Gloo Helm chart archive be present in the `_test` folder. This chart will be used to install 
Gloo to the GKE `kube2e-tests` cluster. 

```bash
make GCLOUD_PROJECT_ID=solo-public BUILD_ID=my-local-build docker build-test-assets
```

RUN_KUBE2E_TESTS=1;DEBUG=1;WAIT_ON_FAIL=0

| Name              | Required  | Description |
| ---               |   ---     |    ---      |
| RUN_KUBE2E_TESTS  | Y         | Must be set to 1, otherwise tests will be skipped |
| DEBUG             | N         | Set to 1 for debug log output |
| WAIT_ON_FAIL      | N         | Set to 1 to prevent Ginkgo from cleaning up the Gloo installation in case of failure. Useful to exec into inspect resources created by the test. A command to resume the test run (and clean up)will be logged to output

