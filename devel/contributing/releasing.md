# Introduction

Kgateway maintains **releases through a GitHub-Actions + GoReleaser pipeline**. This guide provides step-by-step
instructions for creating a *minor* or a *patch* release.

## Background

Kgateway uses [Semantic Versioning 2.0.0](https://semver.org/) to communicate the impact of every release
(`MAJOR.MINOR.PATCH`). Artifacts (binaries, images, etc) are built by [GoReleaser](https://goreleaser.com/) and
published by a single [Release workflow](https://github.com/kgateway-dev/kgateway/actions/workflows/release.yaml)
that can be run on demand via `workflow_dispatch`. Each release starts by creating a
[tracking issue](https://github.com/kgateway-dev/kgateway/issues) (see [issue #11406](https://github.com/kgateway-dev/kgateway/issues/11406)
as an example) so that every task is visible and auditable.

## Prerequisites

After confirming that you have permissions to push to the Kgateway repo, set the
environment variables that will be used throughout the release workflow:

```bash
export MINOR=0
export REMOTE=origin
```

If needed, clone the [Kgateway repo](https://github.com/kgateway-dev/kgateway):

```bash
git clone -o ${REMOTE} https://github.com/kgateway-dev/kgateway.git && cd kgateway
```

### Minor Release

If the release branch **does not** exist, create one:
  
- Create a new release branch from the `main` branch. The branch should be named `v2.${MINOR}.x`, for example, `v2.0.x`:

    ```bash
    git checkout -b v2.${MINOR}.x
    ```

- Push the branch to the Kgateway repo:

    ```bash
    git push ${REMOTE} v2.${MINOR}.x
    ```

### Patch Release

A patch release is generated from an existing release branch, i.e. [v2.0.x](https://github.com/kgateway-dev/kgateway/commits/v2.0.x/).
After all the necessary backport pull requests have merged, you can proceed to the next section.

## Publish the Release

Navigate to the [Release workflow](https://github.com/kgateway-dev/kgateway/actions/workflows/release.yaml) page.

Use the "Run workflow" drop-down in the right corner of the page to dispatch a release, then:

- Select the branch to release from
  - Minor release: Select the `main` branch.
  - Patch release: Select the release branch, e.g. `v2.0.x`, that will be patched.
- Enter the version for the release to create, e.g. `v2.0.3`. This will trigger
  the release process and result in a new GitHub release, [v2.0.3](https://github.com/kgateway-dev/kgateway/releases/tag/v2.0.3)
  for example.
- Click on the "validate release" option, which bootstraps an environment from the
  generated artifacts and runs the conformance suite against that deployed environment.
- The release notes must be manually added to contain the bug fixes, features, etc. included in the release.
  This part of the process will be improved once [Issue 11436](https://github.com/kgateway-dev/kgateway/issues/11436)
  is fixed.

## Verification

Verify the release has been published to the [releases page](https://github.com/kgateway-dev/kgateway/releases)
and contains the expected assets.

## Test

Follow the [quickstart guide](https://kgateway.dev/docs/quickstart/) to ensure the
steps work using the new release. **Note:** You need to manually replace the current version with the new version until
the documentation is updated in the next step.

## Update Documentation

The Kgateway documentation must be updated to reference the new version.

If needed, clone the [Kgateway.dev repo](https://github.com/kgateway-dev/kgateway.dev):

```bash
git clone -o $REMOTE https://github.com/kgateway-dev/kgateway.dev.git && cd kgateway.dev
```

Bump the Kgateway version used by the docs. The following is an example of bumping from v2.0.3 to 2.0.4:

```bash
sed -i '' '1s/^2\.0\.3$/2.0.4/' assets/docs/versions/n-patch.md
```

Sign, commit, and push the changes to the Gateway API Inference Extension repo.

```shell
FORK=<name_of_my_fork>
git commit -s -m "Bumps Kgateway release version"
git push $FORK
```

Submit a pull request to merge the changes from your fork to the kgateway.dev upstream.

## Update Downstreams

The following projects consume Kgateway and should be updated or an issue created to reference
the new release (not required for a patch release):

- Create an issue and submit a pull request to [Inference Gateway](https://github.com/kubernetes-sigs/gateway-api-inference-extension)
  to bump the Kgateway version. See [PR 1094](https://github.com/kubernetes-sigs/gateway-api-inference-extension/pull/1094) as an example.
  **Note** The [getting started](https://gateway-api-inference-extension.sigs.k8s.io/guides/) guide should be tested with the new Kgateway
  version before submitting the PR.

- Create an issue and submit a pull request to [llm-d-infra](https://github.com/llm-d-incubation/llm-d-infra) to bump the Kgateway version.
  See [PR 146](https://github.com/llm-d-incubation/llm-d-infra/pull/146) as an example. **Note** The [quickstart](https://github.com/llm-d-incubation/llm-d-infra/tree/main/quickstart) guide should be tested with the new Kgateway version before submitting the PR.
