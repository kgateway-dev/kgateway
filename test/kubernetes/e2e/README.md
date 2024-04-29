# End-to-End Testing Framework

## Testify
We rely on [testify](https://github.com/stretchr/testify/tree/master) to provide the structure for our end-to-end testing. This allows us to decouple where tests are defined, from where they are run.

## TestCluster
A [TestCluster](./test.go) is the structure that manages tests running against a single Kubernetes Cluster.

Its sole responsibility is to create [TestInstallations](#testinstallation).

## TestInstallation
A [TestInstallation](./test.go) is the structure that manages a group of tests that run against an installation of Gloo Gateway, within a Kubernetes Cluster.

We try to define a single `TestInstallation` per file in a `TestCluster`. This way, it is easy to identify what behaviors are expected for that installation.

## Features
We define all tests in the [features](./features) package. This is done for a variety of reasons:
1. We group the tests by feature, so it's easy to identify which behaviors we assert for a given feature.
2. We can invoke that same test against different `TestInstallation`s. This means we can test a feature against a variety of installation values, or even against OSS and Enterprise installations.

## Thanks
### Inspiration
This framework was inspired by the following projects:
- [Kubernetes Gateway API](https://github.com/kubernetes-sigs/gateway-api/tree/main/conformance)
- [Gloo Platform](https://github.com/solo-io/gloo-mesh-enterprise/tree/main/test/e2e)

### Areas of Improvement
> **Help Wanted:**
> This framework is not feature complete, and we welcome any improvements to it.

Below are a set of known areas of improvement. The goal is to provide a starting point for developers looking to contribute. There are likely other improvements that are not currently captured, so please add/remove entries to this list as you see fit:
- **Debug Improvements**: On test failure, we should emit a report about the entire state of the cluster. This should be a CLI utility as well.
- **Curl assertion**: We need a re-usable way to execute Curl requests against a Pod, and assert properties of the response.
- **Improved install action(s)**: We rely on the [SoloTestHelper](/test/kube2e/helper/install.go) currently, and it would be nice if we relied directly on Helm or Glooctl.
- **Cluster provisioning**: We rely on the [setup-kind](/ci/kind/setup-kind.sh) script to provision a cluster. We should make this more flexible by providing a configurable, declarative way to do this.
- **Istio action**: We need a way to perform Istio actions against a cluster.
- **Argo action**: We need an easy utility to perform ArgoCD commands against a cluster.