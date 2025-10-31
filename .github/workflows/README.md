# Github Workflows

## [Kgateway Conformance Tests](./regression-tests.yaml)
Conformance tests a pinned version of the [Kubernetes Gateway API Conformance suite](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/conformance_test.go).

### Draft Pull Requests
This Github Action will not run by default on a Draft Pull Request. After a Pull Request is marked as `Ready for Review`
it will trigger the action to run.

## [Kubernetes End-to-End Tests](./pr-kubernetes-tests.yaml)
Regression tests run the suite of [Kubernetes End-To-End Tests](https://github.com/kgateway-dev/kgateway/tree/main/test/e2e).

### Draft Pull Requests
This Github Action will not run by default on a Draft Pull Request. After a Pull Request is marked as `Ready for Review`
it will trigger the action to run.

## [Lint Helm Charts](./lint-helm.yaml)
Perform linting on project [Helm Charts](../../install/helm/README.md).

## Comments That Trigger Workflows
- Commenting `/retest` (without any other text) on a PR will trigger the [retest](./retest.yaml) job (limited to kgateway org members only). This will re-run any failed jobs from the latest workflow runs on the PR.
- Commenting `/merge` (without any other text) on a PR will trigger the [enable auto-merge](./automerge.yaml) job (limited to kgateway org members only). This will enable auto-merge for the PR.
    - Note: if all _required_ checks have passed and the PR has been approved, this will immediately add the PR to the merge queue, regardless of whether there are still outstanding/failing _non-required_ checks.
- Commenting `/unmerge` (without any other text) on a PR will trigger the [disable auto-merge](./automerge.yaml) job (limited to kgateway org members only). This will disable auto-merge for the PR.
    - Note: this can only disable auto-merge if the PR isn't already in the merge queue. If the PR is already in the merge queue and you need to remove it, please ask a maintainer to manually remove it from the merge queue.

## Labels That Prevent Merge
The [check-labels](./check-labels.yaml) workflow will block a PR from merging if the PR contains a `do-not-merge*` or `work in progress` label. These labels can be added to a PR to prevent accidental merges.
