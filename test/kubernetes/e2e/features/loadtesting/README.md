# KGateway Load Testing Framework

This directory contains the KGateway load testing framework that implements performance tests based on the gateway-api-bench methodology. The framework focuses on **control plane performance** testing rather than data plane traffic.

## Overview

The load testing framework provides:

- **Attached Routes Test**: Measures Gateway API route attachment performance
- **VCluster Simulation**: Creates fake cluster resources to simulate production-scale environments
- **Scale-Aware Testing**: Automatically adjusts thresholds based on route count (1000 vs 5000+ routes)
- **Performance Monitoring**: Tracks setup time, teardown time, and status propagation

## Prerequisites

Before running the load tests, you need a properly configured Kubernetes cluster with:

- Gateway API CRDs installed
- KGateway controller deployed

## 🚀 Setup Options

```bash
# Navigate to the project root
cd <project-root>

# Create a kind cluster with registry (if you don't have one already)
ctlptl create cluster kind --name kind-kind --registry=ctlptl-registry

# Build KGateway images and load them into the cluster
# This also installs Gateway API CRDs and KGateway
VERSION=1.0.0-ci1 CLUSTER_NAME=kind make kind-build-and-load

# Start Tilt for development workflow
tilt up
```

### Verification

After setup, verify your cluster is ready:

```bash
# Check if Gateway API CRDs are installed
kubectl get crd gateways.gateway.networking.k8s.io

# Check if KGateway controller is running
kubectl get pods -n kgateway-system

# Verify cluster context
kubectl config current-context
```

## Running Tests

### Command Line

```bash
# Run baseline test (1000 routes)
make run-load-tests-baseline

# Run production test (5000 routes)
make run-load-tests-production

# Run all load tests (baseline + production)
make run-load-tests
```

### VS Code Debug Configuration

Add this configuration to your `.vscode/launch.json` file:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "AttachedRoutes Load Test",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/test/kubernetes/e2e/tests/kgateway_test.go",
            "args": [
                "-test.run",
                "^TestKgateway$/^AttachedRoutes$",
                "-test.v"
            ],
            "env": {
                "SKIP_INSTALL": "true",
                "CLUSTER_NAME": "kind",
                "INSTALL_NAMESPACE": "kgateway-system"
            }
        },
        {
            "name": "AttachedRoutes Baseline Only",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/test/kubernetes/e2e/tests/kgateway_test.go",
            "args": [
                "-test.run",
                "^TestKgateway$/^AttachedRoutes$/^TestAttachedRoutesBaseline$",
                "-test.v"
            ],
            "env": {
                "SKIP_INSTALL": "true",
                "CLUSTER_NAME": "kind",
                "INSTALL_NAMESPACE": "kgateway-system"
            }
        },
        {
            "name": "AttachedRoutes Production Only",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/test/kubernetes/e2e/tests/kgateway_test.go",
            "args": [
                "-test.run",
                "^TestKgateway$/^AttachedRoutes$/^TestAttachedRoutesProduction$",
                "-test.v"
            ],
            "env": {
                "SKIP_INSTALL": "true",
                "CLUSTER_NAME": "kind",
                "INSTALL_NAMESPACE": "kgateway-system"
            }
        }
    ]
}
```

#### Environment Variables Explained:

- `SKIP_INSTALL=true`: Skips KGateway installation (assumes it's already installed)
- `CLUSTER_NAME=kind`: Targets the kind cluster named "kind"
- `INSTALL_NAMESPACE=kgateway-system`: Specifies where KGateway is installed

## Test Types and Metrics

### Baseline Test (1000 routes)

- **Purpose**: Tests performance with moderate scale
- **Thresholds**: Setup <30s, Teardown <10s
- **Batch Size**: 100 routes per batch

### Production Test (5000 routes)

- **Purpose**: Tests performance at production scale
- **Thresholds**: Setup <90s, Teardown <20s
- **Batch Size**: 500 routes per batch

### Key Metrics Measured

- **Setup Time**: Time to add 1 incremental route to existing baseline
- **Route Ready Time**: Time until route accepts traffic
- **Teardown Time**: Time to remove 1 route
- **Total Writes**: Number of status updates during test
- **Resource Usage**: CPU, memory, and API call metrics

## Framework Architecture

### Components

- **`types.go`**: Data structures and configuration thresholds
- **`vcluster_simulator.go`**: Simulates fake cluster resources (nodes, services, endpoints)
- **`loadtest_manager.go`**: Orchestrates test execution and resource management
- **`attachedroutes_suite.go`**: Implements the Attached Routes performance test

### Test Methodology

1. Create simulated cluster with appropriate scale
2. Set up test infrastructure (namespaces, services, gateways)
3. Create baseline routes (1000 or 5000) in batches
4. Wait for all routes to be attached to gateways
5. **Start stopwatch** → Add 1 incremental route
6. Measure time until route is ready and status propagates
7. **Stop stopwatch** → Record performance metrics
8. Measure teardown time for incremental route cleanup

## The "Attached Routes" Test Methodology

This test follows a specific methodology inspired by gateway-api-bench and implements the exact approach recommended by the KGateway team for measuring real-world Gateway API performance.

### Phase-by-Phase Breakdown

1. **Simulation Setup**: Create fake cluster with appropriate scale using VCluster simulator
2. **Infrastructure Setup**: Set up test namespaces and backend services
3. **Gateway Creation**: Create and wait for gateways to be ready
4. **Baseline Routes**: Create N routes (1000 or 5000) in batches with grace periods
5. **Translation Wait**: Wait for all routes to be "attached" to gateways (status propagation)
6. **Route Validation**: Verify baseline routes are accepted and valid
7. **Monitoring Start**: Begin watching gateway status changes with event-driven handlers
8. **STOPWATCH START**: Create 1 additional incremental route
9. **Route Ready**: Curl the gateway until it returns 200 (real traffic validation)
10. **STOPWATCH STOP**: Record "User Time" - the actual time users would experience
11. **Teardown**: Delete the incremental route and measure teardown time

### Why This Methodology

- **Incremental Testing**: Adding 1 route to 1000 existing routes tests real-world scenarios
- **Control Plane Focus**: Measures how fast KGateway processes configuration changes
- **Real Traffic Validation**: Uses curl probing instead of unreliable metrics
- **Status Propagation**: Tests how quickly gateway status reflects route changes
- **Realistic Load**: Tests performance under production-like conditions with simulated backends