# Gateway API Conformance Tests

End-to-end tests for kgateway built on the upstream
[Gateway API conformance framework](https://github.com/kubernetes-sigs/gateway-api/tree/main/conformance)
(`sigs.k8s.io/gateway-api/conformance`).


## Prerequisites

The tests require a running cluster with kgateway and the Gateway API CRDs
installed. The simplest setup uses `make run` from the repository root:

```bash
make run

# ARM Mac users:
CLOUD_PROVIDER_KIND=true make run
```

The tests read the active kubeconfig via controller-runtime
(`config.GetConfig`), so the usual `KUBECONFIG` environment variable or
`~/.kube/config` selection applies.

## Running the tests

All variations use the `e2e` build tag and run from the repository root.

### Run all conformance tests (both features)

```bash
go test -tags e2e -v -timeout 10m ./test/sigs_gateway_api_framework/...
```

### Run a single feature

Use `-run` with the path to the feature subtest. The test name matches
`TestE2EConformanceFramework/<feature-name>`:

```bash
# Only basicrouting
go test -tags e2e -v -timeout 10m \
  -run TestE2EConformanceFramework/basicrouting \
  ./test/sigs_gateway_api_framework/...

# Only header_modifiers
go test -tags e2e -v -timeout 10m \
  -run TestE2EConformanceFramework/header_modifiers \
  ./test/sigs_gateway_api_framework/...
```

### Run a single scenario

Append the scenario `ShortName` to the path:
`TestE2EConformanceFramework/<feature-name>/<ShortName>`:

```bash
# Only the GatewayWithRoute scenario
go test -tags e2e -v -timeout 10m \
  -run TestE2EConformanceFramework/basicrouting/GatewayWithRoute \
  ./test/sigs_gateway_api_framework/...

# Only the RequestHeaderModifier scenario
go test -tags e2e -v -timeout 10m \
  -run TestE2EConformanceFramework/header_modifiers/RequestHeaderModifier \
  ./test/sigs_gateway_api_framework/...
```

### Run a single sub-case within a scenario

Each scenario uses `t.Run` for sub-cases (e.g. per-port, per-path). Append
the sub-case name to the path:

```bash
# Only the /add path of RequestHeaderModifier
go test -tags e2e -v -timeout 10m \
  -run TestE2EConformanceFramework/header_modifiers/RequestHeaderModifier//add \
  ./test/sigs_gateway_api_framework/...
```

## How tests are orchestrated

`TestE2EConformanceFramework` in `main_test.go` is the single entry point. Each
run:

1. Constructs a `ConformanceTestSuite` via `common.NewConformanceSuite`.
2. Applies shared base resources (`testdata/gateway.yaml`,
   `testdata/echo-service.yaml`) once with cleanup registered via
   `t.Cleanup`.
3. Verifies the GatewayClass is `Accepted`.
4. Iterates each feature's exported `Tests` slice and runs every scenario
   as a parallel subtest, all sharing the same suite.

Per-scenario manifests (e.g. HTTPRoutes) are applied automatically when
`tc.Run(t, suite)` runs — there is no manual `kubectl apply` step
anywhere.

## Cleanup behaviour

The framework registers a `t.Cleanup` hook for every resource it creates,
which fires in LIFO order when the corresponding test ends:

- Per-scenario HTTPRoutes are deleted when each scenario subtest finishes.
- Shared base resources (Gateway, Service, Pod, Namespace) are deleted
  when `TestE2EConformanceFramework` itself ends.
- If a test fails or panics, Go's `t.Cleanup` still runs the teardowns.

## Adding a new feature

1. Create `features/<feature-name>/` with a non-`_test.go` file
   (e.g. `<feature-name>.go`) declaring:
   - `var ManifestFS embed.FS` (with `//go:embed testdata`).
   - `var Tests = []confsuite.ConformanceTest{ ... }`.
   - One or more exported `confsuite.ConformanceTest` values.
2. Add `testdata/<feature-name>-<scenario>.yaml` with the per-scenario
   resources. Use the feature name as a prefix to avoid manifest path
   collisions across features.
3. Prefix Kubernetes resource names with the feature name
   (e.g. `<feature-name>-route`) to avoid collisions in the shared
   namespace.
4. Import the new package in `main_test.go` and add it to the
   `features` slice.
