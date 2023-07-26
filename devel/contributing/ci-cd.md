# CI/CD

## Pull Request

### Changelog Bot
[Changelog Bot](https://github.com/solo-io/changelog-bot) ensures that changelog entries are valid

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
