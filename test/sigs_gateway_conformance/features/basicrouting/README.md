# Basic Routing Tests using Gateway API conformance framework

This directory contains a POC for using the upstream
[Gateway API conformance framework](https://github.com/kubernetes-sigs/gateway-api/tree/main/conformance)
(`sigs.k8s.io/gateway-api/conformance`) to write end-to-end tests against kgateway.

## What this POC validates

`TestGatewayWithRoute` exercises a single HTTPRoute attached to a Gateway with
two HTTP listeners (ports 80 and 8080) and verifies that requests to
`example.com` are routed to the echo backend on each listener port.

## Test structure

- `main_test.go` - bootstraps a `confsuite.ConformanceTestSuite` from a
  kubeconfig and embeds the test manifests via `embed.FS`.
- `routing_test.go` - declares one `confsuite.ConformanceTest`, runs it through
  `test.Run(t, suite)`, which auto-applies the test's `Manifests` and registers
  teardown hooks via `t.Cleanup`.
- `testdata/` - manifests applied by the framework.
  - `gateway.yaml` - Namespace + Gateway with two HTTP listeners.
  - `backend.yaml` - echo-basic Service + Pod.
  - `gateway-with-route.yaml` - HTTPRoute attached to the Gateway.

## How manifest lifecycle works

The conformance framework's `Applier.MustApplyWithCleanup` creates resources
and registers a `t.Cleanup` callback that deletes them when the test ends.
The base manifests (`gateway.yaml`, `backend.yaml`) are applied from
`applyBaseManifests(t)` inside the test; the test-specific HTTPRoute is
applied automatically by `ConformanceTest.Run` because it is listed in
`Manifests`. There is no manual `kubectl apply` or teardown.

## Prerequisites

This POC assumes the kgateway control plane and Gateway API CRDs are already
installed in the target cluster. The simplest setup is `make run` from the
repository root, which provisions a kind cluster, installs CRDs and MetalLB,
and deploys kgateway.

```bash
make run
# ARM Mac users:
CLOUD_PROVIDER_KIND=true make run
```

## Running the test

From the repository root:

```bash
go test -tags e2e -v -timeout 5m \
  -run TestGatewayWithRoute \
  ./test/sigs_gateway_conformance/features/basicrouting/...
```

The test reads the active kubeconfig via controller-runtime
(`config.GetConfig`), so the usual `KUBECONFIG` environment variable or
`~/.kube/config` selection applies.

## Cleanup behaviour

- Base manifests (gateway, backend) are deleted when the parent test
  function returns.
- The test-specific HTTPRoute manifest is deleted when its `t.Run` subtree
  finishes.
- If the test fails or panics, Go's `t.Cleanup` still runs the teardowns.

## Why a separate directory from `test/e2e_sigs/`

Each POC demonstrates a different framework. Keeping them in independent
directories (with no shared assertions, helpers, or manifests) makes it easy
to compare the two approaches. Code reuse, if desired, is a deliberate next
step rather than an accident of layout.
