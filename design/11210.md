# EP-11210: KGateway Load Testing Framework

* Issue: [#11210](https://github.com/kgateway-dev/kgateway/issues/11210)

<!-- toc -->
* [EP-11210: KGateway Load Testing Framework](#ep-11210-kgateway-load-testing-framework)
  * [Background](#background)
    * [Gateway API Performance Concepts](#gateway-api-performance-concepts)
  * [Motivation](#motivation)
  * [Goals](#goals)
  * [Non-Goals](#non-goals)
  * [Implementation Details](#implementation-details)
    * [Core Test Scenarios](#core-test-scenarios)
      * [Attached Routes Test](#attached-routes-test)
      * [Route Probe Test](#route-probe-test)
      * [Route Change Test](#route-change-test)
    * [Gateway-API-Bench Integration](#gateway-api-bench-integration)
    * [Enhanced Scale Testing](#enhanced-scale-testing)
    * [CI/CD Integration](#cicd-integration)
      * [Test Registration](#test-registration)
      * [Makefile Targets](#makefile-targets)
    * [Test Plan](#test-plan)
      * [Performance Baselines](#performance-baselines)
  * [Alternatives](#alternatives)
    * [Alternative 1: Testify E2E Test Integration (PROPOSED)](#alternative-1-testify-e2e-test-integration-proposed)
    * [Alternative 2: Standalone Load Testing Tool](#alternative-2-standalone-load-testing-tool)
  * [Open Questions](#open-questions)
    * [Performance Thresholds and Baselines](#performance-thresholds-and-baselines)
    * [Test Coverage and Scenarios](#test-coverage-and-scenarios)
    * [CI/CD Integration Strategy](#cicd-integration-strategy)
    * [Technical Implementation](#technical-implementation)
<!-- /toc -->

## Background

This EP proposes adding a comprehensive load testing framework for KGateway to ensure performance and reliability under various load conditions. The framework will integrate into KGateway's existing e2e test infrastructure and is based on proven patterns from the [gateway-api-bench](https://github.com/howardjohn/gateway-api-bench) benchmarking repository.

This enhancement addresses GitHub issue [#11210](https://github.com/kgateway-dev/kgateway/issues/11210), which requests automated scale tests for kgateway that can run in developer environments and as part of build/release workflows. The existing legacy performance test workflows need to be replaced with a modern, integrated solution.

Performance testing is essential for Gateway API implementations as they handle critical traffic routing decisions. Current testing focuses primarily on functional correctness, leaving performance characteristics largely unvalidated. A comprehensive load testing framework will help identify performance bottlenecks, ensure scalability, and prevent regressions in production deployments.

### Gateway API Performance Concepts

The Gateway API model involves several key performance characteristics that this framework will test:

* **HTTPRoute Attachment**: A single HTTPRoute may be attached to many Gateways simultaneously
* **Gateway Status Updates**: The Gateway object has a status field with `attachedRoutes` which stores a count of the total successfully attached route objects
* **Setup Time**: Time after the last route is created until the `attachedRoutes` status is updated to the total route count
* **Teardown Time**: Time after the last route is deleted until the status is reset back to zero

The proposed framework will provide standardized performance testing patterns that can be consistently applied across KGateway development cycles, enabling reliable performance validation and regression detection both locally and in CI/CD pipelines.

## Motivation

Performance testing is crucial for Gateway API implementations as they handle critical traffic routing decisions. The existing Gateway API conformance tests only cover basic functionality, leaving performance characteristics largely untested. Real-world deployments require gateways that can:

1. Handle thousands of routes without performance degradation
2. Maintain traffic availability during configuration changes
3. Provide consistent response times under varying load conditions
4. Scale control plane operations efficiently

Without comprehensive load testing, KGateway risks encountering production performance issues, including memory leaks, route propagation delays, and traffic disruptions during route changes.

## Goals

The following list defines goals for this EP:

* Implement a comprehensive load testing framework for KGateway based on the proven gateway-api-bench pattern
* Integrate load testing into KGateway's existing testify-based e2e test infrastructure
* Provide three core test scenarios: Attached Routes, Route Probe, and Route Change tests
* Enable automated performance regression testing in CI/CD pipelines
* Generate detailed performance metrics and reports for analysis
* Establish performance baselines and alerts for regression detection

## Non-Goals

The following list defines non-goals for this EP:

* Provide production traffic load testing capabilities
* Support non-Gateway API load testing scenarios
* Create a general-purpose load testing framework for arbitrary applications
* Support real-time traffic analysis or APM capabilities

## Implementation Details

### Core Test Scenarios

Following the gateway-api-bench pattern, implemented as testify test cases:

#### Attached Routes Test

* **Purpose**: Measure time for route attachment status to be reflected in Gateway status
* **Implementation**:
  * testify test that monitors Gateway `attachedRoutes` status using Kubernetes client
  * Apply HTTPRoutes via KGateway's existing test helpers
  * Track timing between route creation and status update
* **Metrics**: Route attachment time, status propagation delay, total writes
* **Success Criteria**: All routes attached within configurable time threshold

#### Route Probe Test

* **Purpose**: Measure route propagation time from configuration to traffic acceptance
* **Implementation**:
  * testify test that applies HTTPRoutes sequentially via test framework
  * Use HTTP client to probe endpoints until 200 responses
  * Track propagation latency and error rates
* **Metrics**: Route propagation latency, error count, maximum response time
* **Success Criteria**: Routes become traffic-ready within acceptable latency bounds

#### Route Change Test

* **Purpose**: Ensure traffic continuity during route configuration changes
* **Implementation**:
  * testify test that generates continuous HTTP traffic using test helpers
  * Apply route configuration changes during traffic
  * Monitor control plane behavior during configuration changes
* **Metrics**: Control plane stability, configuration propagation time, API server response times during changes
* **Success Criteria**: Control plane maintains responsiveness during route configuration changes

### Gateway-API-Bench Integration

The implementation includes a framework for comparative analysis against gateway-api-bench benchmarks:

* **Integrated Benchmark Tests**: Run gateway-api-bench benchmarks as part of KGateway's test suite
* **Comparative Metrics Reporting**: Generate reports comparing KGateway performance against benchmarks
* **Regression Detection**: Identify performance regressions relative to benchmark results

### Enhanced Scale Testing

In addition to the core scenarios, the framework supports enhanced scale testing to evaluate KGateway performance under high-load conditions:

* **Large-Scale Route Management**: Test gateway performance with thousands of routes
* **High-Concurrency Traffic Simulation**: Generate and manage high levels of concurrent traffic
* **Resource Utilization Monitoring**: Track CPU, memory, and network usage during tests

### CI/CD Integration

#### Test Registration

```go
// In test/kubernetes/e2e/tests/kgateway_tests.go
func KubeGatewaySuiteRunner() e2e.SuiteRunner {
    // ...existing code...
    kubeGatewaySuiteRunner.Register("LoadTesting", load_testing.NewTestingSuite)
    // ...existing code...
}
```

#### Makefile Targets

```makefile
.PHONY: load-test
load-test:
    TEST_PKG=./test/kubernetes/e2e/features/load_testing make test

.PHONY: load-test-ci
load-test-ci:
    SKIP_INSTALL=true TEST_PKG=./test/kubernetes/e2e/features/load_testing make test
```

### Test Plan

#### Performance Baselines

* Establish baseline performance metrics for KGateway control plane operations
* Focus on control plane scalability rather than data plane performance
* Performance tests will be run on a single-node kind cluster (exact specs TBD, reference: 16-core AMD 9950x CPU with 96GB RAM from gateway-api-bench testing)
* Automated regression detection comparing current vs. baseline results
* Performance trend analysis over time
* Alert thresholds for CI/CD failures
* Metrics collection for KGateway-specific control plane components

## Alternatives

### Alternative 1: Testify E2E Test Integration (PROPOSED)

* **Pros**:
  * Proven automation and testing methodology adapted from gateway-api-bench patterns
  * Integrates seamlessly with KGateway's existing testify-based e2e test infrastructure
  * Runs automatically as part of established CI/CD pipelines
  * Comprehensive test coverage for Gateway API specific scenarios
  * Real-time monitoring and detailed metrics collection
  * Configurable thresholds and performance scoring
  * Consistency with existing KGateway e2e test patterns and team expertise
* **Cons**: Required adapting gateway-api-bench shell script patterns to Go testify framework
* **Decision**: **PROPOSED** - Leverages proven patterns within existing infrastructure

### Alternative 2: Standalone Load Testing Tool

* **Pros**:
  * Could provide more specialized load testing features
  * Independent of KGateway's test infrastructure
  * Could support multiple gateway implementations
* **Cons**:
  * Would require separate CI/CD integration
  * No integration with existing KGateway test utilities
  * Additional maintenance overhead
  * Less alignment with established testing patterns
* **Decision**: Rejected - Integration benefits outweigh standalone advantages

## Open Questions

The following questions remain open for further discussion and resolution:

### Performance Thresholds and Baselines

1. **Setup and Teardown Time Targets**:
   * **Setup time baseline**: 12s (based on gateway-api-bench results)
   * **Setup time threshold**: Under 30s (with regression alert and test failure if exceeded)
   * **Teardown time baseline**: 16s (based on gateway-api-bench results)
   * **Teardown time threshold**: Under 30s (with regression alert and test failure if exceeded)
   * **Route count**: 1000 routes for baseline measurements
   * Framework should measure actual performance and report when taking longer than expected
   * Both setup and teardown use the same 30s threshold for consistency
   * **Threshold scaling**: Start with single gateway performance test in first iteration, add multi-gateway setup in follow-up with potentially higher thresholds
   * **Multi-gateway scenarios**: Will be tested separately with dedicated thresholds (to be determined in follow-up work)

2. **Production Scale Requirements**:
   * **Target production scale**: 50 namespaces with 100 routes each (5,000 routes total) - following gateway-api-bench "large cluster" scenario
   * **Baseline testing**: Start with 1000 routes to establish baselines, then scale to 5,000 routes
   * **Batched thresholds**: If using batch apply approach, thresholds will be per batch of applied routes (per 100 routes, e.g., ≤ 6s/100 routes - actual ranges to be determined through testing)
   * **Threshold scaling**: Thresholds should scale based on route count for different test scenarios
   * **Multi-gateway scenarios**: Will be tested separately with dedicated thresholds (to be determined in follow-up work)

3. **Resource and Concurrency Limits**:
   * Use a single-node kind cluster to minimize testing noise and ensure consistent results
   * Testing environment: single-node kind cluster on standard GitHub runner (ubuntu-22.04) for CI
   * Reference hardware from gateway-api-bench: 16-core AMD 9950x CPU with 96GB RAM
   * Concurrency and API server rate limits to be determined through CI testing

### Test Coverage and Scenarios

1. **Monitoring and Metrics Collection**:
   * **KGateway control plane metrics** should be collected during load tests:
     * `kgateway_collection_transforms_total` - Total number of collection transforms performed
     * `kgateway_collection_transform_duration_seconds` - Duration of collection transform operations
     * `kgateway_collection_resources` - Number of resources in collections
     * `kgateway_status_syncer_resources` - Resources processed by status syncer
   * Monitor control plane CPU and memory usage of KGateway components
   * Data plane CPU/memory usage monitoring will be addressed in follow-up work
   * API server response times refer to Kubernetes API server latency for resource operations (create/update/delete HTTPRoutes, Gateway status updates)
   * Setup and teardown times are direct measurements of API server performance under load

### CI/CD Integration Strategy

1. **Pipeline Integration**:
   * Load tests will run only on specific triggers: releases and nightly builds
   * This approach balances performance validation with CI resource efficiency
   * PR-level testing will continue to focus on functional correctness via existing e2e tests
   * What's the acceptable test duration for CI environments?
   * Should we have different test profiles (quick vs comprehensive)?
   * How do we handle test environment variability across CI systems?

2. **Environment Adaptation**:
   * Should tests adapt automatically to cluster capacity?
   * Do we need to test on different Kubernetes versions?
   * Should we test with different kgateway configurations?
   * How should performance thresholds be configurable per environment?

### Technical Implementation

1. **Framework Architecture**:
   * Simulation framework: Use [pilot-load](https://github.com/howardjohn/pilot-load) approach to simulate large-scale clusters without scheduling pods onto physical machines
   * This involves creating fake nodes and simulating backend services/pods to test control plane performance without resource overhead
   * Focus on control plane testing: HTTPRoute creation, Gateway status updates, API server response times
   * **Operations approach**: Use batched operations (apply 100 routes at a time) rather than bulk operations (applying all routes simultaneously) for more fine-grained measurements and observability
   * Start with a single worker for route creation/deletion, then add concurrent workers as needed
   * Use standard create operations for route management (server-side apply thresholds to be determined through experimentation)

2. **Baseline Establishment and Regression Detection**:
   * Initial performance baselines based on gateway-api-bench results from John's blog:
     * **Setup time**: 17s (target threshold: under 30s)
     * **Teardown time**: 28s (target threshold: under 30s)
     * **Total Writes**: 15 (significantly lower than other implementations)
   * **CPU/Memory thresholds**: Based on relative consumption compared to other gateways
     * **CPU**: 6.6x relative consumption baseline
     * **Memory**: 6.2x relative usage baseline
   * **Data plane performance baselines** (for reference, not primary focus):
     * Single connection: ~36,794 QPS, P50: 0.028ms, P90: 0.030ms, P99: 0.043ms
     * 16 connections: ~170,469 QPS, P50: 0.085ms, P90: 0.144ms, P99: 0.227ms
   * Patch apply operation thresholds to be determined through experimentation

3. **Result Persistence and Reporting**:
   * Performance tests will integrate with existing e2e test reporting infrastructure
   * Local testing: users can use a Grafana dashboard (similar to [pilot-load dashboard](https://github.com/howardjohn/pilot-load/blob/master/install/dashboard.json)) to view results
   * Additional result persistence and reporting features will be addressed in follow-up tasks.
