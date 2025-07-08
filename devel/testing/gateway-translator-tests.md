# Gateway Translator Tests Guide

This guide explains how to work with gateway translator tests in kgateway, including creating new test cases, managing golden files, and troubleshooting common issues.

## Overview

Gateway translator tests validate that Gateway API resources (Gateways, HTTPRoutes, etc.) are correctly translated into Envoy xDS configuration. These tests use a "golden file" approach where expected outputs are stored as files and compared against actual test results.

## Test Structure

Gateway translator tests are located in:

- **Test file**: `/internal/kgateway/translator/gateway/gateway_translator_test.go`
- **Input files**: `/internal/kgateway/translator/gateway/testutils/inputs/`
- **Expected outputs**: `/internal/kgateway/translator/gateway/testutils/outputs/`

### Test Case Structure

Test case follows this pattern:

```go
Entry("Test description", translatorTestCase{
    inputFile:  "subfolder/input-file.yaml",      // Input YAML with K8s resources
    outputFile: "subfolder/expected-output.yaml", // Expected Envoy configuration
    gwNN: types.NamespacedName{                   // Gateway to test
        Namespace: "default",
        Name:      "example-gateway",
    },
    assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
        // Optional: Custom assertions for status reports
    },
}),
```

## Adding a New Test Case

### Step 1: Create Input Files

1. Navigate to `/internal/kgateway/translator/gateway/testutils/inputs/`
2. Create a new subfolder for your test (e.g., `backendconfigpolicy/`)
3. Create a YAML file with your Kubernetes resources:

### Step 2: Add Test Entry

Add your test case to `gateway_translator_test.go`:

```go
Entry("Backend Config Policy with TLS and SAN verification", translatorTestCase{
    inputFile:  "backendconfigpolicy/tls-san.yaml",
    outputFile: "backendconfigpolicy/tls-san.yaml",
    gwNN: types.NamespacedName{
        Namespace: "default",
        Name:      "example-gateway",
    },
}),
```

### Step 3: Run the Test

Run your specific test to generate the golden file:

#### Using Make (Recommended)

```bash
# Run specific test with make
REFRESH_GOLDEN=true GINKGO_USER_FLAGS="--focus='TLS and SAN' --fail-on-pending=false" make test TEST_PKG=./internal/kgateway/translator/gateway
```

#### Using Go Test Directly

```bash
# Run specific test with go test directly
REFRESH_GOLDEN=true go test ./internal/kgateway/translator/gateway -v -ginkgo.focus="TLS and SAN"
```

**Note**: The `REFRESH_GOLDEN=true` environment variable is required to generate golden files. Without it, the test will fail if no expected output file exists.

The test framework will automatically create the expected output file if it doesn't exist.

## Working with Golden Files

### What are Golden Files?

Golden files contain the expected Envoy xDS configuration that should be generated from your input. They serve as the "gold standard" for test validation.

### Automatic Generation

When you run a test for the first time, if no output file exists, the framework automatically:

1. Runs the translation
2. Captures the actual output
3. Saves it as the expected output file
4. Marks the test as passing

### Manual Regeneration

To update golden files (when you've made changes to the translator):

```bash
# Regenerate all golden files using make
REFRESH_GOLDEN=true make test TEST_PKG=./internal/kgateway/translator/gateway

# Regenerate all golden files using go test
REFRESH_GOLDEN=true go test ./internal/kgateway/translator/gateway -v

# Regenerate specific test using make
REFRESH_GOLDEN=true GINKGO_USER_FLAGS="--focus='TLS and SAN' --fail-on-pending=false" make test TEST_PKG=./internal/kgateway/translator/gateway

# Regenerate specific test using go test
REFRESH_GOLDEN=true go test ./internal/kgateway/translator/gateway -v -ginkgo.focus="TLS and SAN"
```

### Common Ginkgo Focus Patterns

- Focus on specific test: `--focus="exact test name"`
- Focus on test category: `--focus="Backend Config Policy"`
- Focus with regex: `--focus="TLS.*SAN"`
- Skip tests: `--skip="azure"`

## Examples

See existing tests for reference:

- Basic HTTP routing: `testutils/inputs/http-routing/`
- TLS configuration: `testutils/inputs/backendconfigpolicy/tls-san.yaml`
- Traffic policies: `testutils/inputs/traffic-policy/`
- Error cases: `testutils/inputs/http-routing-invalid-backend/`

This testing approach ensures that Gateway API resources are correctly translated to Envoy configuration and helps prevent regressions as the codebase evolves.
