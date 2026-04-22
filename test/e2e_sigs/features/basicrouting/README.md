# Basic Routing Tests using sigs/e2e-framework

This directory contains a POC for migrating kgateway's end-to-end tests to use the [sigs/e2e-framework](https://github.com/kubernetes-sigs/e2e-framework) from the Kubernetes community.

## Test Structure

- **main_test.go** - Initializes the test environment via TestMain
- **routing_test.go** - Contains the actual test cases
- **testdata/** - Test-specific Kubernetes manifests (HTTPRoute)
- **../common/testdata/** - Shared gateway, namespace, and backend used by all e2e_sigs tests

## What These Tests Validate

The tests validate basic HTTP routing through a kgateway gateway instance:

1. **TestGatewayWithRoute** - Tests that HTTP requests to a gateway are correctly routed to backend services on both high (8080) and low (80) port listeners with individual assessment steps

---

## Getting Started

### 1. Set up the complete environment

Run `make run` from the repository root to set up everything needed:

```bash
make run
```

**Note for ARM Mac users:** Add `CLOUD_PROVIDER_KIND=true` if running on ARM architecture:

```bash
CLOUD_PROVIDER_KIND=true make run
```

This single command handles all infrastructure setup. It may take several minutes the first time.

### 2. Apply test resources

Deploy the shared gateway/backend and test-specific routes to the cluster:

```bash
# Apply common shared infrastructure (one-time, used by all tests)
kubectl apply -f test/e2e_sigs/features/common/testdata/gateway.yaml
kubectl apply -f test/e2e_sigs/features/common/testdata/backend.yaml

# Apply test-specific route
kubectl apply -f test/e2e_sigs/features/basicrouting/testdata/gateway-with-route.yaml
```

**What this does:**
- Creates a shared Gateway with two listeners (ports 80 and 8080)
- Creates a shared echo backend Service and Pod for all tests to use
- Creates this test's HTTPRoute that routes example.com to the backend

Wait for resources to be ready:

```bash
kubectl wait --for=condition=ready pod/echo-server -n kgateway-test --timeout=60s
kubectl wait --for=condition=accepted gateway/test-gateway -n kgateway-test --timeout=60s
```

### 3. Run the tests

#### Option 1: Using make (Recommended)

From the repository root:

```bash
make e2e-test TEST_PKG=./test/e2e_sigs/features/basicrouting
```

#### Option 2: Using go test directly

Change to the test directory and run:

```bash
cd test/e2e_sigs/features/basicrouting
go test -v -timeout 60s ./... -tags=e2e -kubeconfig=$HOME/.kube/config
```

Run a specific test:

```bash
cd test/e2e_sigs/features/basicrouting
go test -v -timeout 30s -run TestGatewayWithRoute ./... -tags=e2e -kubeconfig=$HOME/.kube/config
```

---

## Test Resources

### Shared Resources (test/e2e_sigs/features/common/testdata/)

- **gateway.yaml** - Defines:
  - `kgateway-test` Namespace
  - A Gateway (`test-gateway`) with two listeners (ports 80 and 8080)

- **backend.yaml** - Defines:
  - A Service (`echo-backend`) that proxies to the echo server
  - A Pod (`echo-server`) running `registry.k8s.io/gateway-api/echo-basic`

### Test-Specific Resources (testdata/)

- **gateway-with-route.yaml** - Defines:
  - An HTTPRoute (`basicrouting-route`) that routes example.com to the shared backend service

All resources must be applied to the cluster before running tests. The tests assume they are already installed and do not create/destroy them.

---

## Architecture & Design

### Key Components

1. **sigs/e2e-framework** - Provides test lifecycle management (Setup, Assess, Teardown)
2. **features.New()** - Creates a named test feature
3. **.Setup()** - Runs once before assessments to initialize state (e.g., get gateway address)
4. **.Assess()** - Individual test step that validates behavior
5. **assertions package** - Reusable assertion helpers to avoid duplication
6. **Shared Gateway Pattern** - Common gateway/backend resources used by multiple tests

### Test Flow

1. TestMain initializes the sigs/e2e-framework environment from kubeconfig
2. Each test feature:
   - Runs Setup to fetch the shared gateway address from the cluster
   - Stores the gateway in context for use by assessments
   - Runs multiple Assess steps (each testing specific ports/scenarios)
   - Implicitly cleans up context when done

### Shared vs Test-Specific Resources

This test reuses a shared Gateway and backend pod defined in `../common/testdata/`:
- **Gateway** (`test-gateway`) - Shared by all e2e_sigs tests
- **Backend** (`echo-server`) - Shared by all e2e_sigs tests
- **HTTPRoute** (`basicrouting-route`) - Test-specific, defines routing rules for this test


## Debugging

To debug a specific test with verbose output:

```bash
cd test/e2e_sigs/features/basicrouting
go test -timeout 60s -run TestGatewayWithRoute -v ./... -tags=e2e -kubeconfig=$HOME/.kube/config
```

Useful commands for troubleshooting:

```bash
# Check gateway has been assigned an address
kubectl get gateway test-gateway -n kgateway-test -o jsonpath='{.status.addresses[0].value}'

# Verify echo backend is running
kubectl get pod echo-server -n kgateway-test
kubectl logs echo-server -n kgateway-test

# Check all test resources
kubectl get gateway,httproute,service,pod -n kgateway-test
```




