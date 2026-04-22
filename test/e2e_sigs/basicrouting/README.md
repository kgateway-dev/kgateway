# Basic Routing Tests using sigs/e2e-framework

This directory contains a POC for migrating kgateway's end-to-end tests to use the [sigs/e2e-framework](https://github.com/kubernetes-sigs/e2e-framework) from the Kubernetes community.

## Test Structure

- **main_test.go** - Initializes the test environment via TestMain
- **routing_test.go** - Contains the actual test cases
- **assertions/assertions.go** - Centralized assertion helpers
- **testdata/** - Kubernetes manifests used in tests (for reference and documentation)

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
This single command handles all infrastructure setup. It may take several minutes the first time.

### 2. Apply test resources

Deploy the test Gateway, HTTPRoute, and backend Service to the cluster:

```bash
kubectl apply -f test/e2e_sigs/basicrouting/testdata/gateway-with-route.yaml
kubectl apply -f test/e2e_sigs/basicrouting/testdata/service.yaml
```

**What this does:**
- Creates a Gateway with two listeners (ports 80 and 8080)
- Creates an HTTPRoute that routes example.com to the backend
- Deploys an echo Service and Pod to handle backend requests

Wait for resources to be ready:

```bash
kubectl wait --for=condition=ready pod/basicrouting-echo -n kgateway-test --timeout=60s
kubectl wait --for=condition=accepted gateway/basicrouting-gateway -n kgateway-test --timeout=60s
```

### 3. Run the tests

#### Option 1: Using make (Recommended)

From the repository root:

```bash
make e2e-test TEST_PKG=./test/e2e_sigs/basicrouting
```

#### Option 2: Using go test directly

Change to the test directory and run:

```bash
cd test/e2e_sigs/basicrouting
go test -v -timeout 60s ./... -tags=e2e -kubeconfig=$HOME/.kube/config
```

Run a specific test:

```bash
cd test/e2e_sigs/basicrouting
go test -v -timeout 30s -run TestGatewayWithRoute ./... -tags=e2e -kubeconfig=$HOME/.kube/config
```

---

## Test Resources

The `testdata/` folder contains the Kubernetes manifests:

- **gateway-with-route.yaml** - Defines:
  - A Gateway (`basicrouting-gateway`) with two listeners (ports 80 and 8080)
  - An HTTPRoute (`basicrouting-route`) that routes example.com to the backend service
  - Resources are created in the `kgateway-test` namespace

- **service.yaml** - Defines:
  - `kgateway-test` Namespace
  - A Service (`basicrouting-backend`) that proxies to the echo server
  - A Pod running `registry.k8s.io/gateway-api/echo-basic` for simple request/response testing

These resources must be applied to the cluster before running tests. The tests assume they are already installed and do not create/destroy them.

---

## Architecture & Design

### Key Components

1. **sigs/e2e-framework** - Provides test lifecycle management (Setup, Assess, Teardown)
2. **features.New()** - Creates a named test feature
3. **.Setup()** - Runs once before assessments to initialize state (e.g., get gateway address)
4. **.Assess()** - Individual test step that validates behavior
5. **assertions package** - Reusable assertion helpers to avoid duplication

### Test Flow

1. TestMain initializes the sigs/e2e-framework environment from kubeconfig
2. Each test feature:
   - Runs Setup to fetch the gateway address from the cluster
   - Stores the gateway in context for use by assessments
   - Runs multiple Assess steps (each testing specific ports/scenarios)
   - Implicitly cleans up context when done


## Debugging

To debug a specific test with verbose output:

```bash
cd test/e2e_sigs/basicrouting
go test -timeout 60s -run TestGatewayWithRoute -v ./... -tags=e2e -kubeconfig=$HOME/.kube/config
```

Useful commands for troubleshooting:

```bash
# Check gateway has been assigned an address
kubectl get gateway basicrouting-gateway -n kgateway-test -o jsonpath='{.status.addresses[0].value}'

# Verify echo backend is running
kubectl get pod basicrouting-echo -n kgateway-test
kubectl logs basicrouting-echo -n kgateway-test

# Check all test resources
kubectl get gateway,httproute,service,pod -n kgateway-test
```




