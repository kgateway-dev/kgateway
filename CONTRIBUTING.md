## Contributing to Gloo Edge

Excited about Gloo Edge and want to help make it better? 

At Solo we strive to make the world of microservices, serverless and service mesh available to everyone. 
If you want to help but don't know where to start, let us know, and we'll find something for you.

If you haven't already, make sure you sign up for the [Solo Slack](https://slack.solo.io).

Here are some of the ways you can contribute: 

* [Filing issues](#filing-issues)
* [Improving the documentation](#improving-the-documentation)
* [Small bug fixes](#small-bug-fixes)
* [Big pull requests](#big-prs)
* [Code review guidelines](#code-review-guidelines)


### Filing issues

If you encounter a bug, please file an issue on GitHub. 
If an issue you have is already reported, please add additional information or add a 👍 reaction to indicate your agreement.


### Improving the documentation

The docs for Gloo Edge are built from the contents found in the [`docs/content`](/docs/content) directory of this repository.

Improving the documentation, adding examples or use cases can be the easiest way to contribute to Gloo Edge. If you see a piece of content that can be better, open a PR with an improvement, it doesn't matter how small!

For more detailed guidance on contributing to the documentation, check out the guide on the [docs website](https://docs.solo.io/gloo-edge/latest/contributing).

### Setting up the development environment

Instructions for setting the development environment can be found in the [developer guide](https://docs.solo.io/gloo-edge/latest/guides/dev/setting-up-dev-environment/). 

Useful make commands:

| Command                                                   | Description |
| ---                                                       |   ---      |
| make install-go-tools generated-code -B                   | Makes all generated code which is required to get past CI. |
| make glooctl                                              | Makes glooctl binary and places it in _output/glooctl |
| make build-test-chart                                     | Makes the .tgz helm file that locally-built instances of glooctl require to install gloo |
| make docker TAGGED_VERSION=(version)                      | Builds the docker images needed for the helm charts and tests |
| make clean build-test-assets -B TAGGED_VERSION=v(version) | Builds a zipped helm chart for gloo that is configured to use the specified gloo version. This version must be a valid image in quay. This can include non-standard versions used for testing. |
| make install-go-tools                                     | Updates the go dependencies |

### Small bug fixes

If your bug fix is small (around 20 lines of code) just open a pull request. We will try to merge it as soon as possible, 
just make sure that you include a test that verifies the bug you are fixing.

### Big PRs

This includes:

- Big bug fixes
- New features

For significant changes to the repository, it’s important to settle on a design before starting on the implementation. Reaching out to us early will help minimize the amount of possible wasted effort and will ensure that major improvements are given enough attention.

1. **Open an issue.** Open an issue about your bug in this repo.
2. **Message us on Slack.** Reach out to us to discuss your proposed changes.
3. **Agree on implementation plan.** Write a plan for how this feature or bug fix should be implemented. Should this be one pull request or multiple incremental improvements? Who is going to do each part?
4. **Submit a work-in-progress PR** It's important to get feedback as early as possible to ensure that any big improvements end up being merged. Submit a pull request and label it `work in progress` to start getting feedback.
5. **Review.** At least one Solo team member should sign off on the change before it’s merged. Look at the “code review” section below to learn about what we're looking for.
6. **A Solo team member will merge and release!**

### Code review guidelines

It’s important that every piece of code in Gloo Edge is reviewed by at least one Solo team member familiar with that codebase.

1. **Changelog** Every PR in Gloo Edge needs a changelog entry. For more information about changelogs, see the [readme](https://github.com/solo-io/go-utils/tree/main/changelogutils). 
2. **CI check** A Solo team member needs to kick off the CI process by commenting `/test` on your PR.
3. **Testing** Please write tests for your changes. Bias towards fast / unit testing. 
4. **Comments** The code reviewer may leave comments to discuss changes. Minor preferences are often called out with `nit`. 

### Testing with coverage:

To see coverage, run your tests in the package like so

```
ginkgo -cover && go tool cover -html *.coverprofile
```