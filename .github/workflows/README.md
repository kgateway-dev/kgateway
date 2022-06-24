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

### Issue Comment Directives
There are several directives that can be used to interact with this Github Action via a comment on the github pull request. The comments must be made by a member of the organization that owns the repo.

- A comment containing `/sig-ci` will clear the status and trigger a new build. This is useful for when the CI build has a flake.
- A comment containing `/skip-ci` will mark the status as successful and bypass the CI build. This should be used sparingly in situations where CI is not needed (i.e. a readme update).

### Draft Pull Requests
This Github Action will not run by default on a Draft Pull Request. However, you can use the above issue comment directives to signal CI.

## [Docs Generation](./docs-gen.yaml)
Docs generation builds the docs that power https://docs.solo.io/gloo-edge/latest/, and on pushes to the main branch, deploys those changes to Firebase.

### Issue Comment Directives
There are several directives that can be used to interact with this Github Action via a comment on the github pull request. The comments must be made by a member of the organization that owns the repo.

- A comment containing `/sig-docs` will clear the status and trigger a new build. This is useful for when the CI build has a flake.
- A comment containing `/skip-docs` will mark the status as successful and bypass the CI build. This should be used sparingly in situations where CI is not needed (i.e. a readme update).

### Draft Pull Requests
This Github Action will not run by default on a Draft Pull Request. However, you can use the above issue comment directives to signal a build.