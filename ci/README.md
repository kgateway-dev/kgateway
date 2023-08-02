# Continuous Integration
- [Pull Request](#pull-request)
    - [Changelog Bot](#changelog-bot)
    - [Build Bot](#build-bot)
    - [Github Actions](#github-actions)
    - [Bulldozer](#bulldozer)
    - [Special Labels](#special-labels)
    - [Special Directives to Skip CI](#special-directives-to-skip-ci)

## Pull Request

### Changelog Bot
[Changelog Bot](https://github.com/solo-io/changelog-bot) ensures that changelog entries are valid.

This is enabled as a GitHub App on the project, and if changes are required, please contact the maintainers of the project by [opening an issue](/devel/contributing/issues.md).

### Build Bot
[Build Bot](https://github.com/solo-io/build-bot) runs unit tests for the entire project. 

This is enabled as a Github App on the project, and is configured by the [cloudbuild.yaml](../cloudbuild.yaml) at the root of the project and contains additional configuration in the [cloudbuild](cloudbuild) folder.

### Github Actions
[Github Workflows](/.github/workflows) run tests which rely on Kubernetes clusters.

See the [GitHub docs](https://docs.github.com/en/actions/automating-builds-and-tests/about-continuous-integration#about-continuous-integration-using-github-actions) for more deatils about how GitHub Actions work.

### Bulldozer
[Bulldozer](https://github.com/solo-io/bulldozer) automatically merges PRs when all required status checks are successful and required reviews are provided.

This is enabled as a GitHub App on the project, and if changes are required, please contact the maintainers of the project by [opening an issue](/devel/contributing/issues.md).

### Special Labels
**Keep PR Updated**: When applied, bulldozer keeps the PR up to date with the base branch, by merging any updates into it (Applied by default).

**Work In Progress**: When applied, will prevent bulldozer from merging a PR, even if it has passed all checks.

### Special Directives to Skip CI
*If you use any of these directives, you must explain in the PR body, why this is safe*

**Skip Changelog**: Following the [special comment directives](https://github.com/solo-io/changelog-bot#issue-comment-directives), comment `skip-changelog` on the PR. **This should rarely be used, even small changes should be documented in the changelog.**

**Skip Build-Bot**: Following the [special comment directives](https://github.com/solo-io/build-bot#issue-comment-directives), comment `skip-ci` on the PR.

**Skip Docs Build**: Include `skipCI-docs-build:true` in the changelog entry of the PR.

**Skip Kubernetes E2E Tests**: Include `skipCI-kube-tests:true` in the changelog entry of the PR.