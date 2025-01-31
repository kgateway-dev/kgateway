# Pull Requests

This doc explains the best practices for submitting a pull request to the [kgateway project](https://github.com/kgateway-dev/kgateway).
It should serve as a reference for all contributors, and be useful especially useful to new and infrequent submitters.
This document serves as a [kgateway](https://github.com/kgateway-dev/kgateway) repo extension of the [org-level contribution guidelines](https://github.com/kgateway-dev/community/blob/main/CONTRIBUTING.md)



# Submission Process
Merging a pull request requires the following steps to be completed before the pull request will be merged automatically.
- [Open a pull request](https://help.github.com/articles/about-pull-requests/)
- Pass all [automated tests](automation.md)
- Get all necessary approvals from reviewers and code owners



## Best Practices for Pull Requests
Below are some best practices we have found to help PRs get reviewed quickly

### Follow Project Conventions
* [Coding conventions](conventions.md)

### Smaller Is Better
Small PRs are more likely to be reviewed quickly and thoroughly. If the PR takes **>45 minutes** to review, the review may be less thorough and more likely to miss something important.

#### Use Commits to Tell the Story
Having a series of discrete commits makes it easier to understand the idea of the PR, and break up the review into smaller chunks

When PRs merge in kgateway, they are squashed into a single commit, so it is not necessary to squash your commits before merging.

#### Avoid Squashing Previous Commits and Using Force Pushes
This can make it difficult to understand the history of the PR, and can make it difficult to understand the changes in the future.

#### Separate Features and Generic Fixes
If you encounter cosmetic changes to the project that you wish to improve (e.g. spelling mistakes, formatting, poor names, etc), please submit these as a separate PR.

As always, use your judgment. A few small changes are fine, but if you find yourself making many changes to unrelated files, it is probably best to split them up.

#### Gather Feedback Early
If your changes are large, or you are unsure of the approach, it is best to gather feedback early. This can be done by opening a draft PR, or by asking for feedback in [the CNCF slack channel](https://cloud-native.slack.com/archives/C080D3PJMS4) 

### Comments Matter More Over Time
In your code, if someone might not understand why you did something, they definitely won't remember later. To avoid this, add comments to your code that express the *why*, since the code should express the *how*.

Read up on [GoDoc](https://blog.golang.org/godoc-documenting-go-code) - follow those general rules for comments.

### Test
Almost every PR that changes code, should also change or introduce tests. If for some reason this doesn't apply to your PR, please explain why in the PR body.

If you do not know how to test a give feature, please ask, and we'll be happy to suggest appropriate test cases.

### PR Body Guidelines
The PR body is generally the first place reviewers will look to gather context on the set of proposed changes. As such, we recommend the following:
- Include a description of the change in the PR body, where the *why* is made clear. This makes it easier to understand the change in the future
- Enumerate all changes, even/especially minor ones, so the reviewer knows they are intentional
- Link any relevant Slack conversations or design documents in the PR body so that they are not lost

When a PR merges into the target branch in Gloo Gateway, the changes are squashed into a single commit, whose message is the PR title. As such, it is important to have a clear title that describes the change