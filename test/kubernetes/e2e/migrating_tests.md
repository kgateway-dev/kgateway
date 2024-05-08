# Migrating Kubernetes End-to-End Tests

This new [framework](./test.go) provides a standard way to validate behavior end-to-end using the Gloo Gateway product. It differs from a previous standard that was implemented in our [kube2e](../../kube2e) package.

_This document outlines the ways that developers can migrate tests from that old paradigm into the new framework._

## 1. Identify the feature
As per the [README](./README.md#features), we group our test cases by feature. If the test you are migrating is part of already tested feature, just add the test to that feature package directly. If the test you are migrating is part of a new feature, add a new feature package.

## 2. Identify the TestInstallation(s)
Features in the product are expected to work under a variety of conditions. Therefore, it may be that you need to invoke the feature test suite from different `TestInstallation`. As per the [README](README.md#testinstallation), we group these TestInstallation by file, so that each file runs the set of tests for a single installation.

## 3. Identify the TestCluster
We [load balance](./load_balancing_tests.md) our tests across multiple clusters. Since you have added new tests, it may be worthwhile to re-balance the tests. Most importantly, if you added a new file for a `TestInstallation`, assign it a test cluster.

## 4. Run the tests locally
It is important that all of our end-to-end tests can be run locally, following our [debugging guide](./debugging.md). If there are updates required to that guide, please make them. 