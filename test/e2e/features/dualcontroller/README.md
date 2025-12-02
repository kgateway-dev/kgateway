# Dual Controller E2E Tests

## Overview

This test suite validates the dual controller architecture requirements documented in `AGENTS.md`. It ensures that:

1. Both Envoy and Agentgateway controllers can run side-by-side
2. Controllers respect `GatewayClass.spec.controllerName` (NOT the GatewayClass name)
3. Controllers do NOT process resources belonging to the other controller
4. Enable flags (`EnableEnvoy`, `EnableAgentgateway`) are honored at all layers

## Test Structure

The suite uses a **performance-optimized approach** that minimizes helm operations:
- Single helm install with Envoy enabled (helm chart requires at least one controller enabled)
- Use helm upgrade to change enable flags for each test
- Use fresh Gateway/Route resources for each test to avoid state contamination
- Tests are **independent** and can run in parallel

### Test Cases

#### TestEnvoyOnly: Envoy Controller Only Enabled
**Setup:** `envoy.enabled=true`, `agentgateway.enabled=false`

**Validates:**
- Envoy Gateway (`gatewayClassName: kgateway`) gets provisioned and becomes ready
- Envoy Gateway status shows Accepted and Programmed
- Envoy HTTPRoute status updated with parent ref
- Traffic works through Envoy Gateway
- Deployment uses envoy chart (has `kgateway-proxy` container)
- Agentgateway Gateway (`gatewayClassName: agentgateway`) is NOT provisioned
- Agentgateway Gateway status NOT updated

#### TestAgentgatewayOnly: Agentgateway Controller Only Enabled
**Setup:** `envoy.enabled=false`, `agentgateway.enabled=true`

**Validates:**
- Agentgateway Gateway gets provisioned and becomes ready
- Agentgateway Gateway status shows Accepted and Programmed
- Agentgateway HTTPRoute status updated
- Traffic works through Agentgateway Gateway
- Deployment uses agentgateway chart (has `agent-gateway` container)
- Envoy Gateway is NOT provisioned
- Envoy Gateway status NOT updated

#### TestBothEnabled: Both Controllers Enabled
**Setup:** `envoy.enabled=true`, `agentgateway.enabled=true`

**Validates:**
- Both Gateway types get provisioned
- Both Gateway statuses show Accepted and Programmed
- Both HTTPRoute statuses updated with their respective parent refs
- Traffic works through both Gateways independently
- Correct charts used (envoy vs agentgateway containers)
- Status entries are namespaced by controllerName:
  - Envoy routes have `controllerName: kgateway.dev/kgateway`
  - Agentgateway routes have `controllerName: kgateway.dev/agentgateway`

## Implementation Details

### Manifest Templates

The test uses two reusable YAML templates:
- `testdata/envoy-gateway.yaml` - Template for Envoy Gateways
- `testdata/agw-gateway.yaml` - Template for Agentgateway Gateways

Each template contains placeholders (`GATEWAY_NAME`, `ROUTE_NAME`, `HOSTNAME`) that are replaced dynamically using `transformManifest` for each test.

### Key Assertions

1. **Resource Provisioning:** `EventuallyObjectsExist` / `ConsistentlyObjectsNotExist`
2. **Gateway Status:** `EventuallyGatewayCondition` for Accepted/Programmed
3. **Route Status:** Direct client.Get to inspect `status.parents[]` and `controllerName`
4. **Traffic:** `AssertEventualCurlResponse` to verify end-to-end functionality
5. **Chart Verification:** Check Deployment container names to distinguish envoy vs agentgateway

### Helm Upgrade Helper

The `upgradeHelmWithFlags()` method:
1. Gets local chart path
2. Sets `envoy.enabled` and `agentgateway.enabled` flags (which map to `KGW_ENABLE_ENVOY` and `KGW_ENABLE_AGENTGATEWAY` env vars)
3. Performs helm upgrade
4. Waits for kgateway controller pod to be ready
5. Allows time for reconciliation

## Running the Tests

### Run the entire suite
```bash
go test -v -timeout 30m -tags e2e ./test/e2e/tests/... -run "^TestDualController$"
```

### Run specific test
```bash
./hack/run-e2e-test.sh TestEnvoyOnly
./hack/run-e2e-test.sh TestAgentgatewayOnly
./hack/run-e2e-test.sh TestBothEnabled
```

### With persistence (faster iteration)
```bash
PERSIST_INSTALL=true ./hack/run-e2e-test.sh DualController
```

## Key Files

- [`suite.go`](suite.go) - Main test implementation
- [`types.go`](types.go) - Manifest paths and object metadata
- [`testdata/envoy-gateway.yaml`](testdata/envoy-gateway.yaml) - Envoy Gateway template
- [`testdata/agw-gateway.yaml`](testdata/agw-gateway.yaml) - Agentgateway template

## Related Documentation

- [AGENTS.md](../../../../AGENTS.md) - Dual controller architecture requirements
- [test/e2e/README.md](../../README.md) - E2E testing framework overview
- [devel/testing/writing-tests.md](../../../../devel/testing/writing-tests.md) - Testing best practices

