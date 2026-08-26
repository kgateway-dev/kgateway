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

- Update the OSV security scan workflow branch allowlist in [.github/workflows/osv-scanner.yaml](../../.github/workflows/osv-scanner.yaml) to include the new release branch.
  This workflow only scans an explicit set of branches, so each newly cut release branch must be added to both the scheduled scan matrix and the `workflow_dispatch` branch options.

### Patch Release

A patch release is generated from an existing release branch, i.e. [v2.0.x](https://github.com/kgateway-dev/kgateway/commits/v2.0.x/).
After all the necessary backport pull requests have merged, you can proceed to the next section.

## Publish the Release

Navigate to the [Release workflow](https://github.com/kgateway-dev/kgateway/actions/workflows/release.yaml) page.

Use the "Run workflow" drop-down in the right corner of the page to dispatch a release, then:

- Select the branch to release from
  - Minor release: Select the `main` branch.
  - Patch release: Select the release branch, e.g. `v2.3.x`, that will be patched.
- Enter the version for the release to create, e.g. `v2.0.3`. Must be a `v`-prefixed semver with no
  build metadata (a `+` suffix is rejected because the version doubles as an image/chart tag). This
  will trigger the release process and result in a new GitHub release,
  [v2.0.3](https://github.com/kgateway-dev/kgateway/releases/tag/v2.0.3) for example.
- Optionally enable "Run load tests" to exercise the load suite against the staged artifacts. Its results do not gate the release.
- Leave "Allow missing previous version" unchecked unless the release is deliberately skipping a version.

The workflow builds and stages the images under a non-semver tag, runs the container-structure and
Gateway API conformance checks against that staged copy, and only if they pass does it promote the
images to the published tag and push the charts + create the GitHub release. A dispatch for a version
that already exists (as a git tag or GitHub release) fails fast and refuses to republish, so a completed
release can't be clobbered by a re-run — the re-run errors out instead.

## Generating Release Notes

The Release workflow runs `make release-notes` automatically and the publish job wraps the output with
the welcome header and installation/quickstart footer (from `hack/release-notes/`) before creating the
GitHub release, so no manual step is required when cutting a release.

To preview release notes locally, use the `hack/generate-release-notes.sh` script:

```bash
GITHUB_TOKEN=<your_token> ./hack/generate-release-notes.sh -p v2.0.3 -c v2.1.0
```

The script does the following:

- Finds all PR numbers from commit messages between the two tags
- Fetches PR details via the GitHub API
- Extracts content from `release-note` code blocks in PR descriptions
- Categorizes entries by `kind/` labels (breaking_change, feature, fix, deprecation, documentation, cleanup, install, bump)
- Generates `_output/RELEASE_NOTES.md` by default

Run `./hack/generate-release-notes.sh --help` to see all options.

After running the script, review the generated file for accuracy, then add the content to the GitHub release description.

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
