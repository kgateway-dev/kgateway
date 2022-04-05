# GH Workflows

## Push API Changes to Solo-APIs
 - This workflow is used to open a PR in Solo-APIs which corresponds to a set of changes in Gloo OSS
 - The workflow is run when a Gloo OSS release is published
 - The workflow can be run manually from the "Actions" tab in Github while viewing the Gloo OSS repo
   - The user must specify three arguments, which should take the following values:
   - `Use workflow from`: The branch in Gloo OSS which the generated Solo-APIs PR should mirror
   - `Release Tag Name`: The specific commit hash/tag in Gloo OSS from which the Solo-APIs PR should be generated
   - `Release Branch`: The Solo-APIs branch which the generated PR should target, most likely `master`