# Load Balancing Tests

## Background
### What is the goal?
Each end-to-end test is executed on a running Kubernetes Cluster. If we use a single cluster, and run tests serially, our CI pipeline will take a long time. Therefore, we load balance our tests against a batch of Kubernetes Clusters.

Our goal is to make the most efficient use of our hardware for executing tests in our CI pipeline.

### What was our previous strategy?
Our previous strategy was to group tests by domain:
- [gloo](https://github.com/solo-io/gloo/tree/v1.16.x/test/kube2e/gloo)
- [gateway](https://github.com/solo-io/gloo/tree/v1.16.x/test/kube2e/gateway)
- ...etc

This did not scale well because different domains had a different amount of tests. Therefore, we noticed that it would take some test clusters twice as long to complete the tests as others.

### What is our current strategy?
Our current strategy is to group tests by runtime. 


Each file is provided a build tag:
```go
//go:build cluster_example
```

This build tag is an indication to our CI pipeline for which cluster that tests should be executed against. This allows us to load balance our tests across multiple clusters. _If you forget to define a tag, it will be run against all clusters, so please do not do that_.

## Re-Balancing

### When should it occur?
Re-balancing of tests is intentionally a very easy action, though it shouldn't need to occur often. This should happen if:
- Tests on one cluster are completing well before tests on another cluster
- All clusters are exhausted, and we need to introduce a new cluster into the rotation

### Steps to take
1. Review the recent results from CI
2. Document the results, on the [GitHub action matrix](/.github/workflows/pr-kubernetes-tests.yaml) that runs the tests
3. Adjust the build tags for the tests in a standalone PR (no other changes)
