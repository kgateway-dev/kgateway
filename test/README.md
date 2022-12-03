# End-to-End Tests

## Background

Gloo Edge is built to integrate with a user's environment. By enabling users to select their preferred tools for scheduling, persistence, and security, we must ensure that we have end-to-end tests to validate these scenarios.

We support the following types of end-to-end tests:
- [Envoy end-to-end](./e2e#envoy-end-to-end-tests)
- [Kubernetes end-to-end](./kube2e#kubernetes-end-to-end-tests)
- [Consul/Vault end-to-end](./consulvaulte2e)

## CI
Each test suite may run on different infrastructure. Refer to the README of each test suite for more information.

## Debugging Tests
*TODO: We should move these debug steps to the individual test suites.*

Some of the gloo tests use a listener on 127.0.0.1 rather than 0.0.0.0 and will only run on linux (e.g. fault injection).

If you’re developing on a mac, ValidateBootstrap will not run properly because it uses the envoy binary for validation mode (which only runs on linux). See rbac_jwt_test.go for an example.