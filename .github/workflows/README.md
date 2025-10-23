# Github Workflows

## [Kgateway Conformance Tests](./regression-tests.yaml)
Conformance tests a pinned version of the [Kubernetes Gateway API Conformance suite](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/conformance_test.go).

### Draft Pull Requests
This Github Action will not run by default on a Draft Pull Request. After a Pull Request is marked as `Ready for Review`
it will trigger the action to run.

## [Kubernetes End-to-End Tests](./pr-kubernetes-tests.yaml)
Regression tests run the suite of [Kubernetes End-To-End Tests](https://github.com/kgateway-dev/kgateway/tree/main/test/kubernetes/e2e).

### Draft Pull Requests
This Github Action will not run by default on a Draft Pull Request. After a Pull Request is marked as `Ready for Review`
it will trigger the action to run.

## [Lint Helm Charts](./lint-helm.yaml)
Perform linting on project [Helm Charts](../../install/helm/README.md).

## Comments That Trigger Workflows
- Commenting `/retest` (without any other text) on a PR will trigger the [Re-run failed jobs](./retest.yaml) workflow (limited to kgateway org members only). This will re-run any failed jobs from the latest workflow runs on the PR.
