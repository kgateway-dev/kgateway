# Test Flakes

Tests must account for the eventually consistent nature of Gloo Edge. Writing deterministic end-to-end tests can be challenging, and at times a test will fail in our CI pipeline non-deterministically. We refer to these failures as flakes. We have found the most common flakes occur as a result of:
1. Test pollution: another test in the same suite does not clean up resources
2. Eventual consistency: the test does not wait long enough for resources to be applied and processed correctly

## What to do if a test flakes?
### 1. Identify that it is a flake
The best way to identify that a flake occurred is to run the test locally.

First, we recommend [focusing the test](https://onsi.github.io/ginkgo/#focused-specs) to ensure that no other tests are causing an impact, and following the Ginkgo recommendations for [managing flaky tests](https://onsi.github.io/ginkgo/#repeating-spec-runs-and-managing-flaky-specs).

If running the test alone does not reproduce the error, it is likely the failure is caused by test pollution. Repeat the above process, but this time move the focus one level up, so that other tests which may be creating or deleting resources are also run, repeating as necessary until the whole suite is focussed.

### 2. Triage the flake
If a test failure is deemed to be a flake, we take the following steps:
1. Determine if there is a [GitHub issue](https://github.com/solo-io/gloo/labels/Type%3A%20CI%20Test%20Flake) tracking the existence of that test flake
1. Investigate the flake, timeboxing to a reasonable amount of time (about an hour). Flakes impact the developer experience, and we want to resolve them as soon as they are identified
1. If a solution can not be determined within the timebox, create a GitHub issue to track it
1. If no issue exists, create one and include the `Type: CI Test Flake` [label](https://github.com/solo-io/gloo/labels/Type%3A%20CI%20Test%20Flake). If an issue already exists, add a comment with the logs for the failed run. We use comment frequency as a mechanism for determining frequency of flakes
1. Retry the test (specific steps can be found in a README of each test suite) and comment on the Pull Request with a link to the GitHub issue tracking the flake


## Identifying the cause of a flake
- Add highly verbose, even excessive, debug logging, run the test repeatedly until the flake occurs, use the logging to trace back the buggy behavior