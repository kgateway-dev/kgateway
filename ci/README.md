# Continuous Integration
- [Pull Request](#pull-request)
    - [Changelog Bot](#changelog-bot)
    - [Github Actions](#github-actions)
    - [Special Labels](#special-labels)
    - [Special Directives to Skip CI](#special-directives-to-skip-ci)

## Pull Request

### Github Actions
[Github Workflows](/.github/workflows) run tests which rely on Kubernetes clusters.

See the [GitHub docs](https://docs.github.com/en/actions/automating-builds-and-tests/about-continuous-integration#about-continuous-integration-using-github-actions) for more deatils about how GitHub Actions work.


### Special Directives to Skip CI
*If you use any of these directives, you must explain in the PR body, why this is safe*

// TODO: build out directives based on updated ci