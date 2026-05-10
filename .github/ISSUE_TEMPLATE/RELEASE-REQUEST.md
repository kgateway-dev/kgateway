---
name: Release Request
about: Track the steps to cut a kgateway patch or minor release
title: "Release Request: Cut a v<MAJOR>.<MINOR>.<PATCH> Release"
labels: ["kind/release"]
---

Tracking issue for cutting the `v<MAJOR>.<MINOR>.<PATCH>` release from the `v<MAJOR>.<MINOR>.x` release branch.

## Backports / tracked work

PRs and/or issues that need to land before this release is cut. List the headline items the release is being cut for; it does not need to be exhaustive. No need to list PRs already present on the branch.

- [ ] #
- [ ] #
- [ ] others?

## Prerequisites

- [ ] You are a kgateway maintainer (have push permissions to `kgateway-dev/kgateway`)
- [ ] All required backports are merged to the `v<MAJOR>.<MINOR>.x` release branch (the merge queue wouldn't allow any CI failures)
- [ ] You are aware that nightly tests run nightly only on `main`, and that they are the only thing that runs against a vector of Kubernetes versions and a vector of Gateway API versions.
  - [ ] And you have [run them manually](https://github.com/kgateway-dev/kgateway/actions/workflows/nightly-tests.yaml) against the tip of `v<MAJOR>.<MINOR>.x`, and it passed or had irrelevant failures
  - [ ] Or you have decided not to run them manually
- [ ] osv-scanner's scan from last night is acceptably clean for this branch (see `https://github.com/kgateway-dev/kgateway/security/code-scanning?query=is%3Aopen+branch%3Av<MAJOR>.<MINOR>.x` substituting MAJOR and MINOR). If not, fix things and run the OSV scan GitHub Action manually to confirm that you've fixed things.

### First-time setup for a new minor release branch (skip for patch releases)

- [ ] Create and push the `v<MAJOR>.<MINOR>.x` branch from `main` (see [`devel/contributing/releasing.md`](/devel/contributing/releasing.md))
- [ ] Create a branch protection ruleset, or ask a maintainer to do so, for the `v<MAJOR>.<MINOR>.x` branch
- [ ] Add the new release branch to [`.github/workflows/osv-scanner.yaml`](/.github/workflows/osv-scanner.yaml) (both the scheduled scan matrix and the `workflow_dispatch` branch options)

## Publish the release

The [Release workflow](https://github.com/kgateway-dev/kgateway/actions/workflows/release.yaml) generates the release notes, creates the git tag, builds the artifacts, and publishes the GitHub release. No local steps are needed to cut the release.

- [ ] Open the [Release workflow](https://github.com/kgateway-dev/kgateway/actions/workflows/release.yaml)
- [ ] Run with branch `v<MAJOR>.<MINOR>.x` and version `v<MAJOR>.<MINOR>.<PATCH>`
- [ ] Enable "validate release" (runs the conformance suite against the released artifacts)
- [ ] Watch the workflow to completion
- [ ] Review the generated release notes on the GitHub release; edit the description if anything was miscategorized

## Verify

- [ ] Confirm the tag and assets on the [releases page](https://github.com/kgateway-dev/kgateway/releases)
- [ ] Walk through the [quickstart guide](https://kgateway.dev/docs/quickstart/) using the new version, or at least run `hack/setup-via-release.sh -v v<MAJOR>.<MINOR>.<PATCH>` with the most recent and least recent supported Gateway API versions
- [ ] If the quickstart is broken with the new version, file an issue in [`kgateway-dev/kgateway.dev`](https://github.com/kgateway-dev/kgateway.dev/issues) before announcing

## Update documentation (kgateway.dev)

- [ ] Bump the latest stable patch in `assets/docs/versions/n-patch.md`
- [ ] If applicable, bump the Gateway API version in `assets/docs/versions/k8s-gw-version.md`
- [ ] If patching a non-latest minor (e.g. `v2.0.x` while `v2.1` is current), update the versioned folder/conref for that line — see [kgateway.dev PR #447](https://github.com/kgateway-dev/kgateway.dev/pull/447/files) as a worked example
- [ ] Open and merge a PR to `kgateway-dev/kgateway.dev`

## Close-out

- [ ] Announce the release in the `#kgateway` channel on [CNCF Slack](https://slack.cncf.io/)
- [ ] Close this issue
