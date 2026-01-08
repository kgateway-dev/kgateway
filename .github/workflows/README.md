# Github Workflows

## Pull Request CI Checks

The following checks are required to pass in order for a PR to be merged:

### [DCO](https://developercertificate.org/)
Ensures that each commit contains a `Signed-off-by` trailer to adhere to [DCO](https://developercertificate.org/) requirements. See [Submission Process](/devel/contributing/pull-requests.md#submission-process) for more details.

### [Labeler](./labeler.yaml)
Parses the PR description to extract the change type and changelog for release notes. PR descriptions must adhere to the [Pull Request template](https://github.com/kgateway-dev/.github/blob/main/.github/PULL_REQUEST_TEMPLATE.md).

### [Lint](./lint.yaml)
Checks if there are any linting errors in the Go code, Rust code, Helm charts, or GitHub workflow files.

### [Verify](./verify.yaml)

Runs [code generation](/devel/contributing/code-generation.md) and makes sure that generated files are up to date.

### [Unit Tests](./unit.yaml)

Runs all Go unit tests.

### [Gateway API Conformance Tests](./conformance.yaml)
Runs conformance tests against both the experimental and standard Gateway API channels.
Uses the upstream [Kubernetes Gateway API Conformance suite](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/conformance_test.go).

**Note**: This Github Action will not run by default on a Draft Pull Request.
After a Pull Request is marked as `Ready for Review` it will trigger the action to run.

### [Kubernetes End-to-End Tests](./e2e.yaml)
Runs the suite of [Kubernetes End-To-End Tests](/test/e2e).

**Note**: This Github Action will not run by default on a Draft Pull Request.
After a Pull Request is marked as `Ready for Review` it will trigger the action to run.

## Interacting with CI

### Comments That Trigger Workflows
- Commenting `/retest` (without any other text) on a PR will trigger the [retest](./retest.yaml) job (limited to kgateway org members only). This will re-run any failed jobs from the latest workflow runs on the PR.
- Commenting `/merge` (without any other text) on a PR will trigger the [enable auto-merge](./automerge.yaml) job (limited to kgateway org members only). This will enable auto-merge for the PR.
    - Note: if all _required_ checks have passed and the PR has been approved, this will immediately add the PR to the merge queue, regardless of whether there are still outstanding/failing _non-required_ checks.
- Commenting `/unmerge` (without any other text) on a PR will trigger the [disable auto-merge](./automerge.yaml) job (limited to kgateway org members only). This will disable auto-merge for the PR.
    - Note: this can only disable auto-merge if the PR isn't already in the merge queue. If the PR is already in the merge queue and you need to remove it, please ask a maintainer to manually remove it from the merge queue.

### Labels That Prevent Merge
The [check-labels](./check-labels.yaml) workflow will block a PR from merging if the PR contains a `do-not-merge*` or `work in progress` label. These labels can be added to a PR to prevent accidental merges.
