# In Memory End-to-End tests
These end-to-end tests do not require Kubernetes, and persist configuration in memory.

- [Background](#background)
  - [Where are the tests?](#where-are-the-tests)
  - [How do the tests work?](#how-do-the-tests-work)
  - [Example Test](#example-test)
- [CI](#ci)
- [Local Development](#local-development)

## Background
This is the most common type of end-to-end test, since it is the quickest to set up and easiest to debug. Additionally, since Gloo Edge may be run using various backing stores, these tests provide a single space to validate the translation of Gloo resources into Envoy resources, independent of where Gloo Edge is deployed. As a result, these test do not rely on Kubernetes, so if there is any Kubernetes behavior that needs to be tested, write a [kubernetes end-to-end test](../kube2e) instead.

### Where are the tests?
The tests are located in the [test/e2e](/test/e2e) folder

### How do the tests work?
1. Run the [Gloo controllers in goroutines](https://github.com/solo-io/gloo/blob/1f457f4ef5f32aedabc58ef164aeea92acbf481e/test/services/gateway.go#L109)
1. Run [Envoy](https://github.com/solo-io/gloo/blob/1f457f4ef5f32aedabc58ef164aeea92acbf481e/test/services/envoy.go#L237) either using a binary or docker container
1. Apply Gloo resources using [in-memory resource clients](https://github.com/solo-io/gloo/blob/1f457f4ef5f32aedabc58ef164aeea92acbf481e/test/services/gateway.go#L175)
1. Execute requests against the Envoy proxy and confirm the expected response. This validates that the Gloo resources have been picked up by the controllers, were been translated correctly into Envoy configuration, the configuration was sent to the Envoy proxy, and the proxy behaves appropriately.

### Example Test
We have an [example test](/test/e2e/example_test.go) which outlines how these tests work. It also provides some examples for basic testing operations. If you are writing a new e2e test, we recommend that you review the example test first.

This was introduced in a [pull request](https://github.com/solo-io/gloo/pull/7555) which includes other useful details about e2e test considerations.

## CI
These tests are run by [build-bot](https://github.com/solo-io/build-bot) in Google Cloud as part of our CI pipeline.

If a test fails, you can retry it using the build-bot [comment directives](https://github.com/solo-io/build-bot#issue-comment-directives). If you do this, please make sure to include a link to the failed logs for debugging purposes.

## Local Development
See the [e2e test README](/test/e2e/README.md) for more details about running these tests.