# CI Helper Scripts

This directory contains helper scripts used in GitHub Actions CI workflows.

## validate_e2e_coverage.go

### Purpose

Validates that all e2e test functions are covered by at least one regex pattern in the GitHub Actions workflow file (`.github/workflows/e2e.yaml`).

### Problem It Solves

The e2e CI workflow uses regex-based test selection to distribute tests across multiple cluster configurations. If a developer adds a new e2e test function but forgets to update the regex selection rules in the workflow, the test will never run in CI and no warning is produced.

This script acts as a safety mechanism to catch such cases early.

### How It Works

1. **Discovers all Go test functions** in `test/e2e/tests/` directory
   - Scans all `*_test.go` files
   - Finds functions matching pattern: `func TestXxx(t *testing.T)`

2. **Extracts regex patterns** from `.github/workflows/e2e.yaml`
   - Parses all `go-test-run-regex` values from the workflow matrix

3. **Validates coverage**
   - Checks if each discovered test is matched by at least one regex pattern
   - Handles both exact matches (`^TestName$`) and prefix matches (`^TestName`)

4. **Reports uncovered tests**
   - If any test is not matched, prints an error and exits with status 1
   - This causes the CI job to fail, alerting developers to update the workflow

### Usage

```bash
# Run from repository root
go run hack/ci/validate_e2e_coverage.go
```

### Example Output

**Success:**
```
🔍 Discovering e2e test functions...
   Found 15 test functions

📋 Extracting workflow regex patterns...
   Found 10 regex patterns in workflow

✅ Checking test coverage...

✅ SUCCESS: All e2e tests are covered by workflow regex patterns!
```

**Failure:**
```
🔍 Discovering e2e test functions...
   Found 16 test functions

📋 Extracting workflow regex patterns...
   Found 10 regex patterns in workflow

✅ Checking test coverage...

❌ ERROR: The following e2e tests are NOT covered by any workflow regex:

   - TestNewFeature

💡 Please update the regex patterns in .github/workflows/e2e.yaml
   to include these tests in the appropriate cluster configuration.
```

### CI Integration

This script runs as a separate job (`validate_e2e_coverage`) in the e2e workflow before the actual test execution. The e2e test jobs depend on this validation job, so if validation fails, the tests won't run.

### Maintenance

- **No external dependencies**: Uses only Go standard library
- **Fast execution**: Typically completes in < 1 second
- **Self-contained**: All logic in a single file

### When to Update

You typically don't need to modify this script. However, you may need to update it if:

- The e2e test directory structure changes
- The workflow file format changes significantly
- The regex pattern format in the workflow changes

### Related Documentation

- [E2E Test Load Balancing Guide](../../test/e2e/load_balancing_tests.md)
- [E2E Testing Framework](../../test/e2e/README.md)
