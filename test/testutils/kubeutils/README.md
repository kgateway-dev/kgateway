# Kubeutils
This package contains Kubernetes utilities that are used within our tests.

## Contributing to this package
As a general strategy, we want to avoid polluting this package. If you are considering adding code here, consider the following question:
- Would a user or developer expect to take this action against a running cluster?

If the answer is yes, try to place the code in a more apt utility, like [kubeutils](/pkg/utils/kubeutils).