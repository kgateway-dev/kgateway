# GH Workflows

## [Push API Changes to Solo-APIs](./push-solo-apis-branch.yaml)
 - This workflow is used to open a PR in Solo-APIs which corresponds to a set of changes in Gloo OSS
 - The workflow is run when a Gloo OSS release is published
 - The workflow can be run manually from the "Actions" tab in Github while viewing the Gloo OSS repo
   - Ensure that PRs created from manual workflow runs are not merged by adding the "Work in Progress" tag or by making 
     the PR a draft.
   - The user must specify three arguments, which should take the following values:
   - `Use workflow from`: The branch in Gloo OSS which the generated Solo-APIs PR should mirror
   - `Release Tag Name`: The specific commit hash/tag in Gloo OSS from which the Solo-APIs PR should be generated
   - `Release Branch`: The Solo-APIs branch which the generated PR should target, most likely `master`

## [Regression Tests](./regression-tests.yaml)
Regression tests run the suite of [Kubernetes End-To-End Tests](https://github.com/solo-io/gloo/tree/master/test).

### Draft Pull Requests
This Github Action will not run by default on a Draft Pull Request. If you would like to run this, you need to:
1. Mark the PR as `Ready for Review`
1. Push an empty commit to run the jobs: `git commit --allow-empty -m "Trigger CI"` 

## [Docs Generation](./docs-gen.yaml)
Docs generation builds the docs that power https://docs.solo.io/gloo-edge/latest/, and on pushes to the main branch, deploys those changes to Firebase.

### Draft Pull Requests
This Github Action will not run by default on a Draft Pull Request. If you would like to run this, you need to:
1. Mark the PR as `Ready for Review`
1. Push an empty commit to run the jobs: `git commit --allow-empty -m "Trigger CI"`

## [Trivy Vulnerability Scanning](./trivy-analysis-scheduled.yaml)
A scheduled job which scans images released from both the Open Source and Enterprise repositories.

To run the vulnerability locally, check out [the security scanner README](https://github.com/solo-io/gloo/tree/master/docs/cmd/securityscanutils)

## Future Work
It would be great to add support for issue comment directives. This would mean that commenting `/sig-ci` would signal CI to run, or `/skip-ci` would auto-succeed CI.

This was attempted, and the challenge is that Github workflows were kicked off, but not associated with the PR that contained the comment. Therefore, the PR status never changed, even if the job that was kicked off passed all the tests.