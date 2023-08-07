# Backports

## What is a backport?
A backport is a change that is introduced on the main branch, and then applied to a previous version of Gloo Edge.

## When is backporting appropriate?
For a backport to be appropriate it must fit the following criteria:
- The change must have a clear rationale for why it is needed on a previous version of Gloo Edge.
- The change must be a low-risk change, this often means it is a bug fix or a non-breaking change.
- The proposed change is targeted to a [supported, stable release branch](https://docs.solo.io/gloo-edge/latest/reference/support/).
- If the change is a feature request, you must have explicit approval from the product and engineering teams. This approval can also be solicited on the backport prs themselves

## How to identify a backport
On the issue that tracks the desired functionality, apply a `release/1.N` label to indicate the version of Gloo Edge you wish the request to be supported on.

For example, if there is a `release/1.14` label, that means the issue is targeted to be introduced first on the stable main branch, and then backported to Gloo Edge 1.14.x.

## How to create a backport
First, create a PR to introduce the change on the main branch. Doing so ensures that changes are tested and reviewed before being applied to a previous version of Gloo Edge. Also, given that we use [protocol buffers](https://developers.google.com/protocol-buffers) for our API definitions, introducing API changes to our main branch first ensures we will not have API compatibility issues when backporting.

Once the change has been merged into the main branch, create a PR to backport the change to the desired release branch. The PR should be titled `[BRANCH NAME]: <PR title>` (ie `[1.14]: Original PR Title`). To open the PR you likely need to do the following:
- We use [cherry-pick](https://git-scm.com/docs/git-cherry-pick) to apply changes to a previous version of Gloo Edge.
- Modify the changelog to be in the proper directory
- Validate that Proto fields have the same numbers as in main
- Title your pr to start with the major.minor version that you are backporting to (e.g. 1.13 for 1.13.x branch)