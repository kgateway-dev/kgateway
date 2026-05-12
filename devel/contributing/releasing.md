# Introduction

Kgateway maintains **releases through a GitHub-Actions + GoReleaser pipeline**. This guide provides step-by-step
instructions for creating a *minor* or a *patch* release.

> **Making any changes here?** See if you should update the issue template at
> [.github/ISSUE_TEMPLATE/RELEASE-REQUEST.md](/.github/ISSUE_TEMPLATE/RELEASE-REQUEST.md) to keep its checklist in sync.

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

- Update the OSV security scan workflow branch allowlist in [.github/workflows/osv-scanner.yaml](../../.github/workflows/osv-scanner.yaml) to include the new release branch.
  This workflow only scans an explicit set of branches, so each newly cut release branch must be added to both the scheduled scan matrix and the `workflow_dispatch` branch options.

### Patch Release

A patch release is generated from an existing release branch, e.g. [v2.2.x](https://github.com/kgateway-dev/kgateway/commits/v2.2.x/).
After all the necessary backport pull requests have merged, you can proceed to the next section.

## Publish the Release

Navigate to the [Release workflow](https://github.com/kgateway-dev/kgateway/actions/workflows/release.yaml) page.

Use the "Run workflow" drop-down in the right corner of the page to dispatch a release, then:

- Select the branch to release from
  - Minor release: Select the `main` branch.
  - Patch release: Select the release branch, e.g. `v2.2.x`, that will be patched.
- Enter the version for the release to create, e.g. `v2.0.3`. This will trigger
  the release process and result in a new GitHub release, [v2.0.3](https://github.com/kgateway-dev/kgateway/releases/tag/v2.0.3)
  for example.
- Click on the "validate release" option, which bootstraps an environment from the
  generated artifacts and runs the conformance suite against that deployed environment.
- The workflow automatically generates release notes and publishes them with the GitHub release. If you need to preview them locally, see [Generating Release Notes](#generating-release-notes) below.

The workflow generates release notes automatically (see [Release Notes](#release-notes) below).
Once the workflow completes, review the release notes on the GitHub release and edit the description
if anything was miscategorized.

## Release Notes

The Release workflow runs `make release-notes` automatically and feeds the output to GoReleaser, so no
manual step is required when cutting a release. Under the hood it invokes
[`hack/generate-release-notes.sh`](../../hack/generate-release-notes.sh), which:

- Finds all PR numbers from commit messages between the previous tag and the new release
- Fetches PR details via the GitHub API
- Extracts content from `release-note` code blocks in PR descriptions
- Categorizes entries by `kind/` labels (breaking_change, feature, fix, deprecation, documentation, cleanup, install, bump)

To preview release notes locally — for example to sanity-check what an upcoming release will include —
you can invoke the script directly:

```bash
GITHUB_TOKEN=<your_token> ./hack/generate-release-notes.sh -p v2.0.3 -c v2.1.0
```

This writes `_output/RELEASE_NOTES.md`. Run `./hack/generate-release-notes.sh --help` for all options.

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

### Latest Stable Versions
Bump the Kgateway version used by the docs. The following is an example of bumping from v2.0.3 to v2.0.4:

```bash
sed -i '' '1s/^2\.0\.3$/2.0.4/' assets/docs/versions/n-patch.md
```

Optionally, update the Gateway API version if Kgateway bumps this dependency. The following is an example
of bumping Gateway API from v1.2.1 to v1.3.0:

```bash
GW_API_VERSION=$(cd ../kgateway && go list -m sigs.k8s.io/gateway-api | awk '{print $2}' | sed 's/^v//' && cd ../kgateway.dev)
sed -i '' "1s/.*/${GW_API_VERSION}/" assets/docs/versions/k8s-gw-version.md
```

### Docs for previous Versions
The [kgateway.dev repo](https://github.com/kgateway-dev/kgateway.dev) does not use branches to support documentation for previous versions. It uses versioned folders and conrefs. See [PR 447](https://github.com/kgateway-dev/kgateway.dev/pull/447/files) as an example of updating v2.0.4 to v2.0.5 when v2.1.0 was the latest release.


### Push Changes (All Versions)
Sign, commit, and push the changes.

```shell
FORK=<name_of_my_fork>
git commit -s -m "Bumps Kgateway release version"
git push $FORK
```

Submit a pull request to merge the changes from your fork to the kgateway.dev upstream.

## Update Downstreams

The following projects consume Kgateway and should be updated or an issue created to reference
the new release (not required for a patch release):

- Create an issue and submit a pull request to [llm-d-infra](https://github.com/llm-d-incubation/llm-d-infra) to bump the Kgateway version.
  See [PR 146](https://github.com/llm-d-incubation/llm-d-infra/pull/146) as an example. **Note** The [quickstart](https://github.com/llm-d-incubation/llm-d-infra/tree/main/quickstart) guide should be tested with the new Kgateway version before submitting the PR.
