# Example Suite
_This suite provides a template for writing end-to-end suites. It is not currently executed as part of our CI pipeline_

## ClusterSuite
A `ClusterSuite` captures all of the tests that will be executed against a running Kubernetes Cluster.

## Separate Files per Installation
Each file defines the set of tests that will be executed against an `e2e.TestInstallation`. This is done for the benefit of developers so it is easy to identify all of the tests we author for a given installation.
