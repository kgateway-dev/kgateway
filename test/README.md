# End-to-End Tests

## Background

Gloo Edge is built to integrate with a user's environment. By enabling users to select their preferred tools for scheduling, persistence, and security, we must ensure that we have end-to-end tests to validate these scenarios.

We support the following types of end-to-end tests:
- [In Memory end-to-end](./e2e#in-memory-end-to-end-tests)
- [Kubernetes end-to-end](./kube2e#kubernetes-end-to-end-tests)
- [Consul/Vault end-to-end](./consulvaulte2e)

## CI
Each test suite may run on different infrastructure. Refer to the README of each test suite for more information.