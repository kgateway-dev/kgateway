# Contributing to KGateway

KGateway is an Apache 2.0 licensed project and accepts contributions via GitHub pull requests (PRs). We're excited to have you contribute to making kgateway better!

## Communication Channels

- GitHub Discussions: For miscellaneous discussions
- GitHub Issues: For bugs, feature requests, CI flakes, etc.
- Community Meetings: Check our [community repository](https://github.com/kgateway-dev/community) for meeting schedules

## Find something to work on

The project uses [GitHub issues](https://github.com/kgateway-dev/kgateway/issues) to track bugs and features. Issues labeled with the `good first issue` label are a great place to start.

Additionally, the project has a [milestone](https://github.com/kgateway-dev/kgateway/milestones) for the next release. Any issues labeled with a milestone are a great source of things to work on. If an issue has not been assigned to a milestone, you can ask to work on it by leaving a comment on the issue.

Flaky tests are a common source of issues and a good place to start contributing to the project. If you see a test that is failing regularly, you can leave a comment asking if someone is working on it.

## Contributing Process

### Filing Issues

If you encounter a bug or have a feature request:
1. Search existing issues first
2. If no existing issue addresses your case, create a new one
3. Use issue templates when available
4. Add additional information or 👍 reactions to existing issues

### Code Contributions

#### Small Changes (Bug Fixes)

For small changes (less than 100 lines of code):

1. Open a pull request
2. Ensure tests verify the fix
3. Update documentation if needed

#### Large Changes (Features/Refactoring)

For significant changes:

1. **Open an issue first** - Discuss your proposed changes
2. **Design discussion** - Engage with maintainers on the implementation approach
3. **Implementation plan** - Agree on the implementation strategy
4. **Work-in-progress PR** - Submit a draft PR for early feedback
5. **Review & iterate** - Address feedback from maintainers
6. **Merge** - Once approved, a maintainer will merge

### Code Review Guidelines

All code must be reviewed by at least one maintainer. Key requirements:

1. **Code Style**
   - Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
   - Follow [Effective Go](https://golang.org/doc/effective_go)
   - Run `make analyze` to check for common issues before submitting

2. **Testing**
   - Add unit tests for new functionality
   - Ensure existing tests pass
   - Include integration and e2e tests when needed

3. **Documentation**
   - Update relevant documentation
   - Include code comments for non-obvious logic
   - Update API documentation if changing interfaces

### Documentation Contributions

The project documentation lives in a separate repository: [kgateway-dev/kgateway.dev](https://github.com/kgateway-dev/kgateway.dev). Any developer documentation contributions should be made in that repository (via README.md files).

## Questions?

Don't hesitate to ask questions in:

- GitHub Discussions
- Issue comments
- Pull Request comments

We aim to be a welcoming community and help contributors get started!
