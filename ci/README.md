# Continuous Integration

## Pull Request

### Changelog Bot
[Changelog Bot](https://github.com/solo-io/changelog-bot)  ensures that changelog entries are valid

### Build Bot
[Build Bot](https://github.com/solo-io/build-bot) runs unit tests for the entire project. This is configured with the [cloudbuild.yaml](../cloudbuild.yaml) at the root of the project and contains additional configuration in the [cloudbuild](cloudbuild) folder.

### Github Actions
[Github Workflows](https://github.com/solo-io/gloo/tree/main/.github/workflows) run tests which rely on Kubernetes clusters

### Bulldozer
[Bulldozer](https://github.com/solo-io/bulldozer) automatically merges PRs once all required status checks are successful and required reviews are provided

### Special Labels
**Keep PR Updated**: When applied, bulldozer will keep the PR up to date with the base branch, by merging any updates into it (Applied by default)

**Work In Progress**: When applied, will prevent bulldozer from merging a PR, even if it has passed all checks
### Special Directives to Skip CI
**Skip Build-Bot**: Following the [special comment directives](https://github.com/solo-io/build-bot#issue-comment-directives), comment `skip-ci` on the PR.

**Skip Docs Build**: Include `skipCI-docs-build:true` in the changelog entry of the PR.

**Skip Kubernetes E2E Tests**: Include `skipCI-kube-tests:true` in the changelog entry of the PR.

### What if a test fails on a Pull Request?

Tests must account for the eventually consistent nature of Gloo Edge. Writing deterministic end-to-end tests can be challenging, and at times a test will fail in our CI pipeline non-deterministically. We refer to these failures as flakes. We have found the most common flakes occur as a result of:
1. Test pollution: another test in the same suite does not clean up resources
2. Eventual consistency: the test does not wait long enough for resources to be applied and processed correctly

#### 1. Identify that it is a flake
The best way to identify that a flake occurred is to run the test locally.

First, we recommend [focusing the test](https://onsi.github.io/ginkgo/#focused-specs) to ensure that no other tests are causing an impact, and following the Ginkgo recommendations for [managing flaky tests](https://onsi.github.io/ginkgo/#repeating-spec-runs-and-managing-flaky-specs).

If running the test alone does not reproduce the error, it is likely the failure is caused by test pollution. Repeat the above process, but this time move the focus one level up, so that other tests which may be creating or deleting resources are also run, repeating as necessary until the whole suite is focussed.

#### 2. Triage the flake
If a test failure is deemed to be a flake, we take the following steps:
1. Determine if there is a [GitHub issue](https://github.com/solo-io/gloo/labels/Type%3A%20CI%20Test%20Flake) tracking the existence of that test flake
1. Investigate the flake, timeboxing to a reasonable amount of time (about an hour). Flakes impact the developer experience, and we want to resolve them as soon as they are identified
1. If a solution can not be determined within the timebox, create a GitHub issue to track it
1. If no issue exists, create one and include the `Type: CI Test Flake` label. If an issue already exists, add a comment with the logs for the failed run. We use comment frequency as a mechanism for determining frequency of flakes
1. Retry the test (specific steps can be found in a README of each test suite) and comment on the Pull Request with a link to the GitHub issue tracking the flake

## Codegen

To run the full codegen process, use `make -B install-go-tools generated-code`. In most cases it is not necessary to run all sub-targets within `generated-code`.
Here is a description of each sub-target and its purpose:

| Target                     | Description                                                                                                                    | Use when...                                                                          | Approximate runtime                  |
|----------------------------|:-------------------------------------------------------------------------------------------------------------------------------|:-------------------------------------------------------------------------------------|--------------------------------------|
| `install-go-tools`         | Invokes `mod-download` and `go install`s a number of tools used by `generated-code`                                            | The `_output` dir is not present or you are otherwise unsure deps are installed      | 3-8s (if mod download is up-to-date) |
| `mod-download`             | Calls `go mod download all`                                                                                                    | Deps are not all present locally                                                     | 1s (if deps are already downloaded)  |
| `generated-code`           | Generates all required code, cleaning and formatting as well; this target is executed in CI                                    | You need to run all codegen steps without human error (ie prior to PR merge)         | 4-5m                                 |
| `check-go-version`         | Validates that local Go version matches go.mod                                                                                 | You need a sanity check                                                              | 1s                                   |
| `clean-solo-kit-gen`       | Removes files generated by codegen other than docs and tests                                                                   | An existing generated file may no longer be generated by codegen (rare)              | 3s                                   |
| `go-generate-all`          | Invokes all generate directives in the repo, most notably in `generate.go` which runs solo-kit codegen, and mockgen directives | There is an any API change                                                           | 3-5m                                 |
| `go-generate-apis`         | Invokes the generate directive in `generate.go` which runs solo-kit codegen                                                    | There is a proto API change (prefer to use `generated-code-apis` which also formats) | 1m                                   |
| `go-generate-mocks`        | Invokes all mockgen generate directives in the repo                                                                            | There is an interface API change                                                     | 2:30-3m                              |
| `generate-cli-docs`        | Removes existing CLI docs and re-generates them                                                                                | CLI code has been modified                                                           | 10-15s                               |
| `generated-code-apis`      | Generates and formats code from protos                                                                                         | There is a proto API change                                                          | 1:30s                                |
| `generated-code-cleanup`   | Executes cleanup and formatting targets                                                                                        | Preparing to open a PR without API changes                                           | 30-40s                               |
| `getter-check`             | Ensures that accesses for fields which have "getter" functions are exclusively done via said "getter" functions                | Any code has been written (just in case)                                             | ~7s                                  |
| `mod-tidy`                 | Calls `go mod tidy`                                                                                                            | Dependencies have been added, updated, or removed                                    | 1-2s                                 |                                |
| `verify-enterprise-protos` | Validates that protos used in EE but not OSS are valid                                                                         | Unsure whether protos used in EE are valid                                           | 3-10s                                |
| `generate-helm-files`      | Generates helm chart and value files                                                                                           | The helm chart or values template has been modified                                  | 1-3s                                 |
| `update-licenses`          | Generates docs files containing attribution for all dependencies which require it                                              | There is a new dependency or a depency bump                                          | 7-12s                                |
| `fmt`                      | Runs [`goimports`](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) which updates imports and formats code                 | Code has been modified (any change, in case it's not properly formatted)             | 9-13s                                |