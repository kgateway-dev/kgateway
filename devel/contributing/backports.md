# Backports

## What is a backport?
A backport is a change that is introduced on the main branch, and then applied to a previous version of Gloo Edge.

## When is backporting appropriate?
For a backport to be appropriate it must fit the following criteria:
- The change must have a clear rationale for why it is needed on a previous version of Gloo Edge
- The change must be a bug fix or a non-breaking change
- The proposed change is targed to a [stable release branch](https://docs.solo.io/gloo-edge/latest/reference/support/)
- If the change is feature request, it must have explicit approval from the product and engineering teams

## How to identify a backport
On the issue that tracks the desired functionality, apply a `release/1.N` label to indicate the version of Gloo Edge you wish the request to be supported on.

For example, if there is a `release/1.14` label, that means the issue is targeted to be introduced first on the stable main branch, and then backported to Gloo Edge 1.14.x.
