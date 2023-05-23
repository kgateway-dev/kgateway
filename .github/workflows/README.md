# Github Workflows

## [Push API Changes to Solo-APIs](./push-solo-apis-branch.yaml)
This Github Action is responsible for updating [Solo-Apis](https://github.com/solo-io/solo-apis), the read-only mirror of the Gloo API. 
When this job runs properly, it will push API changes the solo-apis repository, on a branch with the following name:
```
sync-apis/gloo-[GLOO_LTS_BRANCH]/gloo-[GLOO_COMMIT_TO_MIRROR]
```

From there, solo-apis automation will open a Pull Request for that branch.

There are two triggers for this job, and each are explained below:
### 1. On a Release
Whenever Gloo Edge is released, we want to automatically update the published APIs in our read-only mirror. This action will be triggered, and after a release completes a developer needs to manually approve the PR in solo-apis.

### 2. On a Manual Trigger
We may want to test API changes in other projects, without actually merging the API changes into Gloo Edge. To do this, we push the API changes to a branch in Gloo, and then use this workflow to publish the changes to solo-apis.

The arguments are:
- `Use Workflow From`: The branch which contains the workflow code you want to execute. Often times, the default branch will work.
- `The branch that contains the relevant API change`: The branch which contains the API changes you want to mirror
- `The LTS branch that these API changes are targeted for`: The Gloo LTS branch which you want to merge your changes into

Below are some examples for inputs to the job, if you are working on a feature branched named `feature/new-api`

To test this on the default main branch:
- Use Workflow From: `main`
- The branch that contains the relevant API change: `feature/new-api`
- The LTS branch that these API changes are targeted for: `main`

To test this on 1.13.x branch:
- Use Workflow From: `v1.13.x`
- The branch that contains the relevant API change: `feature/new-api`
- The LTS branch that these API changes are targeted for: `v1.13.x`

**NOTE: After the PR opens in solo-apis, we want to avoid the chance that it merges. Please put a 'work in progress' label on the PR to prevent it from merging.**

## [Regression Tests](./regression-tests.yaml)
Regression tests run the suite of [Kubernetes End-To-End Tests](https://github.com/solo-io/gloo/tree/main/test).

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

To run the vulnerability locally, check out [the security scanner README](https://github.com/solo-io/gloo/tree/main/docs/cmd/securityscanutils)

## Future Work
It would be great to add support for issue comment directives. This would mean that commenting `/sig-ci` would signal CI to run, or `/skip-ci` would auto-succeed CI.

This was attempted, and the challenge is that Github workflows were kicked off, but not associated with the PR that contained the comment. Therefore, the PR status never changed, even if the job that was kicked off passed all the tests.