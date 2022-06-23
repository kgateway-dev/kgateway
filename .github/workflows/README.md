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

**This action will not execute on Draft PRs**

## [Docs Generation](./docs-gen.yaml)
Build the docs that power https://docs.solo.io/gloo-edge/latest/, and on pushes to the main branch, deploy those changes to Firebase.

**This action will not execute on Draft PRs**