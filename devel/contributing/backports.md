# Backports

## What is a backport?
A backport is a change that is introduced on the main branch, and then applied to a previous version of kgateway.

## When is backporting appropriate?
For a backport to be appropriate it must fit the following criteria:
- The change must have a clear rationale for why it is needed on a previous version of kgateway.
- The change must be a low-risk change, this typically means it is a bug fix or a non-breaking change.
- Generally a backport for a feature should have a larger discussion in the community and may need to be brought up at a community meeting.

## How to identify a backport
On the issue that tracks the desired functionality, apply a `release/1.N` label to indicate the version of kgateway you wish the request to be supported on.


## How to create a backport
First, create a PR to introduce the change on the main branch. Doing so ensures that changes are tested and reviewed before being applied to a previous version of kgateway. 

Once the change has been merged into the main branch, create a PR to backport the change to the desired release branch. The PR should be titled `[BRANCH NAME]: <PR title>` (ie `[1.14]: Original PR Title`). To create a backport branch we recommend the following:
- Use [cherry-pick](https://git-scm.com/docs/git-cherry-pick) to apply changes to a previous version of kgateway.
  - Resolve any conflicts that have arisen due to drift.
  - If there is significant drift that causes the cherry-pick to be non-trivial, consider re-implementing the change from scratch rather than "backporting"