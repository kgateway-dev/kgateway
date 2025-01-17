# Continuous Integration
- [Pull Request](#pull-request)
    - [Changelog Bot](#changelog-bot)
    - [Github Actions](#github-actions)
    - [Special Labels](#special-labels)
    - [Special Directives to Skip CI](#special-directives-to-skip-ci)

## Pull Request

### Changelog Bot
[Changelog Bot](https://github.com/solo-io/changelog-bot) ensures that changelog entries are valid.

This is enabled as a GitHub App on the project, and if changes are required, please contact the maintainers of the project by [opening an issue](/devel/contributing/issues.md).

### Github Actions
[Github Workflows](/.github/workflows) run tests which rely on Kubernetes clusters.

See the [GitHub docs](https://docs.github.com/en/actions/automating-builds-and-tests/about-continuous-integration#about-continuous-integration-using-github-actions) for more deatils about how GitHub Actions work.

### Special Labels
**Keep PR Updated**: When applied, bulldozer keeps the PR up to date with the base branch, by merging any updates into it (Applied by default).

**Work In Progress**: When applied, will prevent bulldozer from merging a PR, even if it has passed all checks.

### Special Directives to Skip CI
*If you use any of these directives, you must explain in the PR body, why this is safe*

**Skip Changelog**: Following the [special comment directives](https://github.com/solo-io/changelog-bot#issue-comment-directives), comment `/skip-changelog` on the PR. **This should rarely be used, even small changes should be documented in the changelog.**

**Skip Build-Bot**: Following the [special comment directives](https://github.com/solo-io/build-bot#issue-comment-directives), comment `/skip-ci` on the PR.

**Skip Docs Build**: Include `skipCI-docs-build:true` in the changelog entry of the PR.

**Skip Kubernetes E2E Tests**: Include `skipCI-kube-tests:true` in the changelog entry of the PR.

### Assets and Package Management
`glooctl` is built and published to the GitHub release via the script `upload_github_release_assets.go`. This is sensitive to changes to the output of `glooctl version`.

`glooctl` is also available through the package management tool [Homebrew](https://formulae.brew.sh/formula/glooctl). `glooctl` is on the [Autobump list](https://github.com/Homebrew/homebrew-core/blob/8064f66cd04d0f32dc1be25ce8363a7a9e370fae/.github/autobump.txt#L790) which means it is automatically updated within 3 hours of a stable release via [GitHub Actions within Homebrew](https://github.com/Homebrew/homebrew-core/blob/8064f66cd04d0f32dc1be25ce8363a7a9e370fae/.github/workflows/autobump.yml)
