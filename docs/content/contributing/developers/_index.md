---
title: Contribute to the Gloo Edge code
menuTitle: Development
description: Contribute to the codebase for the Gloo Edge project.
weight: 10
---

As a developer, you can contribute code to the [Gloo Edge project](https://github.com/solo-io/gloo).

## Ways to contribute

* [Filing issues](#filing-issues)
* [Small bug fixes](#small-bug-fixes)
* [Big pull requests](#big-prs)
* [Documentation]({{< versioned_link_path fromRoot="/contributing/documentation/" >}})
* [Extending plug-in functionality]({{< versioned_link_path fromRoot="/contributing/extend-edge/" >}})

### Filing issues

If you encounter a bug, you can report the issue on GitHub.

1. Review [existing issues](https://github.com/solo-io/gloo/issues). If you find a similar issue, add a comment with more context or a 👍 reaction to indicate your agreement.
2. If you don't find a similar issue, [open an issue](https://github.com/solo-io/gloo/issues/new/choose) using the appropriate template, such as a **Bug Report**.


### Small bug fixes

If your bug fix is small (around 20 lines of code), just open a pull request. The PR template walks you through providing context and tests that verify your fix works. Solo's engineering team will try to merge the fix as soon as possible.

### Big PRs

Sometimes, you might need to open a larger PR, such as for:

- Big bug fixes
- New features

For significant changes to the Gloo Edge project, get input on the design before starting on the implementation.

1. Refer to [Filing issues](#filing-issues) to find or open an issue with your idea.
2. Message the [Solo team on Slack](https://slack.solo.io) to discuss your proposed changes and come up with an implementation plan.
3. Refer to the [`devel` directory](https://github.com/solo-io/gloo/tree/main/devel) in the Gloo Edge project for tools and helpful information to contribute, debug, and test your code.
4. Open a draft PR with the `wip` label to get feedback on your work.
5. Address any review comments that a Solo team member leaves.

**The Solo team will merge and release your code changes!**

## Code review guidelines

Every piece of code in Gloo Edge is reviewed by at least one Solo team member familiar with that codebase.

1. **Changelog** Every PR in Gloo Edge needs a changelog entry. For more information about changelogs, see the [readme](https://github.com/solo-io/go-utils/tree/main/changelogutils). 
2. **CI check** A Solo team member needs to kick off the CI process by commenting `/test` on your PR.
3. **Testing** Please write tests for your changes. Bias towards fast / unit testing. 
4. **Comments** The code reviewer may leave comments to discuss changes. Minor preferences are often called out with `nit`.

## Testing with coverage

To check coverage, run your tests in the package, such as:

```shell
ginkgo -cover && go tool cover -html *.coverprofile
```
