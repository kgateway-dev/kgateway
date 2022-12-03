# Continuous Integration

## Pull Request

### Changelog Bot
[Changelog Bot](https://github.com/solo-io/changelog-bot)  ensures that changelog entries are valid

### Build Bot
[Build Bot](https://github.com/solo-io/build-bot) runs unit tests for the entire project

### Github Actions
[Github Workflows](https://github.com/solo-io/gloo/tree/master/.github/workflows) run tests which rely on Kubernetes clusters

### Bulldozer
[Bulldozer](https://github.com/solo-io/bulldozer) automatically merges PRs once all required status checks are successful and required reviews are provided

### Special Labels
**Keep PR Updated**: When applied, bulldozer will keep the PR up to date with the base branch, by merging any updates into it (Applied by default)

**Work In Progress**: When applied, will prevent bulldozer from merging a PR, even if it has passed all checks