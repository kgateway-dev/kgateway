# kgateway setup envtests

These tests cover behavior that requires a live Kubernetes control plane, such as Istio integration and update propagation. Pure-translation golden scenarios belong in `../translator/gateway/testutils/inputs/setup` and run through `TestSetupScenarios` in the gateway translator package.

## Adding a live-control-plane scenario

Add a `.yaml` file to the appropriate directory under `testdata`. The scenario must define a Gateway named `http-gw-for-test`; the test renames that Gateway to isolate scenarios. Other resource names must also be unique because these tests do not run in parallel.

The first run creates a sibling `-out.yaml` xDS golden and intentionally fails. Subsequent runs apply the resources, request an xDS snapshot, and compare it with that golden. The dynamically allocated endpoint for the built-in `kubernetes` Service is omitted because its port changes between runs.

## How to run

From the repository root, run:

```shell
go test -tags e2e -v ./pkg/kgateway/setup/
```

## Shared resources

- `testdata/setup_yaml/setup.yaml`: GatewayClass and GatewayParameters
- `testdata/setup_yaml/pods.yaml`: shared Pods and Nodes
- `testdata/istio_crds_setup/crds.yaml`: Istio CRDs

## Scenario directories

- `testdata/serviceentry/dr`: DestinationRules applied to ServiceEntries
- `testdata/istio_destination_rule`: Istio DestinationRule integration
- `testdata/traffic_distribution`: traffic distribution with Istio integration
- `testdata/istio_mtls`: Istio auto-mTLS integration
