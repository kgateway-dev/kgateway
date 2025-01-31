# Github Workflows

## [Gloo Gateway Conformance Tests](./regression-tests.yaml)
Conformance tests a pinned version of the [Kubernetes Gateway API Conformance suite](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/conformance_test.go).

### Draft Pull Requests
This Github Action will not run by default on a Draft Pull Request. After a Pull Request is marked as `Ready for Review`
it will trigger the action to run.

## [Regression Tests](./regression-tests.yaml)
Regression tests run the suite of [Kubernetes End-To-End Tests](https://github.com/solo-io/gloo/tree/main/test).

### Draft Pull Requests
This Github Action will not run by default on a Draft Pull Request. After a Pull Request is marked as `Ready for Review`
it will trigger the action to run.

## [Trivy Vulnerability Scanning](./trivy-analysis-scheduled.yaml)
A scheduled job which scans images released from both the Open Source and Enterprise repositories.

To run the vulnerability locally, check out [the security scanner README](https://github.com/solo-io/gloo/tree/main/docs/cmd/securityscanutils)

## Future Work
It would be great to add support for issue comment directives. This would mean that commenting `/sig-ci` would signal CI to run, or `/skip-ci` would auto-succeed CI.

This was attempted, and the challenge is that Github workflows were kicked off, but not associated with the PR that contained the comment. Therefore, the PR status never changed, even if the job that was kicked off passed all the tests.
