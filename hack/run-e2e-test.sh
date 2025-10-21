#!/usr/bin/env bash

set -eEuo pipefail

# Script to run a single e2e test case
#
# This script intelligently finds and runs e2e test cases using git grep.
# It can run:
#   - Top-level test functions (e.g., TestKgateway)
#   - Entire test suites (e.g., SessionPersistence)
#   - Individual test methods within suites (e.g., TestCookieSessionPersistence)
#
# The script will automatically generate the most specific go test -run pattern.
#
# Environment Variables:
#   PERSIST_INSTALL - If set to true/1/yes/y, will skip 'make setup' if a kind
#                     cluster already exists. This speeds up test runs when you're
#                     iterating locally.
#   AUTO_SETUP      - If set to true/1/yes/y, will automatically clean up conflicting
#                     Helm releases if detected. Otherwise, will error out.
#   CLUSTER_NAME    - Name of the kind cluster (default: kind)
#   TEST_PKG        - Go test package to run (default: ./test/kubernetes/e2e/tests)
#
# Usage: ./hack/run-e2e-test.sh [OPTIONS] [TEST_PATTERN]
#
# Options:
#   --rebuild, -r   Delete the kind cluster, rebuild all docker images, create a
#                   new kind cluster, load images into kind, and then run tests.
#                   This ensures a completely fresh environment.
#   --list, -l      List all available test suites and top-level tests
#   --dry-run, -n   Print the test command that would be run without executing it
#
# Examples:
#   # Run an entire test suite
#   ./hack/run-e2e-test.sh SessionPersistence
#
#   # Run a specific test method within a suite
#   ./hack/run-e2e-test.sh TestCookieSessionPersistence
#
#   # Run a top-level test function
#   ./hack/run-e2e-test.sh TestKgateway
#
#   # Skip setup if cluster exists (faster iteration)
#   PERSIST_INSTALL=true ./hack/run-e2e-test.sh SessionPersistence
#
#   # Auto-cleanup conflicting Helm releases
#   AUTO_SETUP=true ./hack/run-e2e-test.sh SessionPersistence
#
#   # Delete cluster and rebuild everything from scratch
#   ./hack/run-e2e-test.sh --rebuild SessionPersistence
#
#   # Use a different cluster name
#   CLUSTER_NAME=my-cluster ./hack/run-e2e-test.sh SessionPersistence
#
#   # Print the test command without running it
#   ./hack/run-e2e-test.sh -n TestCookieSessionPersistence

# Get the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

# Check if KIND is available
KIND="${KIND:-kind}"
CLUSTER_NAME="${CLUSTER_NAME:-kind}"

log_info() {
    echo -e "INFO: $*" >&2
}

log_success() {
    echo -e "SUCCESS: $*" >&2
}

log_warn() {
    echo -e "WARNING: $*" >&2
}

log_error() {
    echo -e "ERROR: $*" >&2
}

# Check if an environment variable is truthy (1, true, yes, y)
is_truthy() {
    local val="${!1:-}"
    [[ "${val,,}" =~ ^(1|true|yes|y)$ ]]
}

# Check if kind cluster exists
kind_cluster_exists() {
    ${KIND} get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"
}

# Check for and handle conflicting Helm releases
# Returns: 0 if no cleanup needed, 1 if cleanup was performed
check_and_cleanup_helm_conflicts() {
    # Only check if kubectl is available and cluster exists
    if ! command -v kubectl &> /dev/null; then
        return 0
    fi

    if ! kubectl cluster-info &> /dev/null; then
        return 0
    fi

    log_info "Checking for existing kgateway installations..."

    # When AUTO_SETUP is enabled, clean up everything for a fresh start
    if is_truthy AUTO_SETUP; then
        log_info "AUTO_SETUP is enabled, cleaning up all kgateway installations..."

        # Uninstall Helm releases from common namespaces
        for ns in gloo-system kgateway-system kgateway-test default; do
            for release in gloo-gateway-crds kgateway-crds gloo-gateway kgateway; do
                if helm list -a -n "$ns" 2>/dev/null | grep -q "^${release}"; then
                    log_info "Uninstalling Helm release '${release}' from namespace '${ns}'..."
                    helm uninstall "${release}" -n "${ns}" 2>/dev/null || true
                fi
            done
        done

        # Delete ALL kgateway CRDs
        log_info "Deleting all kgateway CRDs..."
        local crds_list
        crds_list=$(kubectl get crds 2>/dev/null | grep 'kgateway.dev' | awk '{print $1}' || true)
        if [[ -n "$crds_list" ]]; then
            while IFS= read -r crd_name; do
                if [[ -n "$crd_name" ]]; then
                    log_info "Deleting CRD: ${crd_name}"
                    kubectl delete crd "${crd_name}" --ignore-not-found=true 2>/dev/null || true
                fi
            done <<< "$crds_list"
        fi

        # Wait a moment for deletions to complete
        sleep "${SLEEP_FOR_DELETE:-2}"

        log_info "Cleanup complete, will run 'make setup' to reinstall"
        echo ""
        return 1  # Signal that cleanup happened and setup is needed
    fi

    # If AUTO_SETUP is not enabled, just check and return
    log_info "No cleanup performed (set AUTO_SETUP=true to enable automatic cleanup)"
    return 0
}

# Search for test cases
find_test_cases() {
    local pattern="$1"

    log_info "Searching for test cases matching: ${pattern}"

    # Search for:
    # 1. Top-level test functions: func TestXxx(t *testing.T)
    # 2. Suite registrations: Register("SuiteName", ...)
    # 3. Suite test methods: func (s *...) TestXxx()

    # Try to find suite registrations first
    local suite_matches
    suite_matches=$(git grep -n "Register(\"${pattern}\"" -- 'test/kubernetes/e2e/tests/*.go' 2>/dev/null || true)

    # Try to find test functions (handle both TestXxx and Xxx patterns)
    local func_matches
    if [[ "$pattern" == Test* ]]; then
        func_matches=$(git grep -n "func ${pattern}" -- 'test/kubernetes/e2e/tests/*_test.go' 2>/dev/null || true)
    else
        func_matches=$(git grep -n "func Test${pattern}" -- 'test/kubernetes/e2e/tests/*_test.go' 2>/dev/null || true)
    fi

    # Try to find test methods in suites (handle both TestXxx and Xxx patterns)
    local method_matches
    if [[ "$pattern" == Test* ]]; then
        method_matches=$(git grep -n "func (.*) ${pattern}" -- 'test/kubernetes/e2e/features' 2>/dev/null || true)
    else
        method_matches=$(git grep -n "func (.*) Test${pattern}" -- 'test/kubernetes/e2e/features' 2>/dev/null || true)
    fi

    # Also try pattern as substring
    if [[ -z "$suite_matches" && -z "$func_matches" && -z "$method_matches" ]]; then
        suite_matches=$(git grep -n "Register(\".*${pattern}.*\"" -- 'test/kubernetes/e2e/tests/*.go' 2>/dev/null || true)

        # For partial match, search for Test.*pattern
        if [[ "$pattern" == Test* ]]; then
            func_matches=$(git grep -n "func .*${pattern}" -- 'test/kubernetes/e2e/tests/*_test.go' 2>/dev/null || true)
            method_matches=$(git grep -n "func (.*) .*${pattern}" -- 'test/kubernetes/e2e/features' 2>/dev/null || true)
        else
            func_matches=$(git grep -n "func Test.*${pattern}" -- 'test/kubernetes/e2e/tests/*_test.go' 2>/dev/null || true)
            method_matches=$(git grep -n "func (.*) Test.*${pattern}" -- 'test/kubernetes/e2e/features' 2>/dev/null || true)
        fi
    fi

    if [[ -z "$suite_matches" && -z "$func_matches" && -z "$method_matches" ]]; then
        log_error "No test cases found matching: ${pattern}"
        echo ""
        log_info "Available test suites:"
        git grep -h 'Register("' -- 'test/kubernetes/e2e/tests/*.go' | \
            sed -E 's/.*Register\("([^"]*)".*/  - \1/' | sort | head -20
        echo ""
        log_info "Available top-level tests:"
        git grep -h '^func Test.*\(t \*testing\.T\)' -- 'test/kubernetes/e2e/tests/*_test.go' | \
            sed -E 's/func (Test[^(]*).*/  - \1/' | sort | head -20
        exit 1
    fi

    # Display findings
    if [[ -n "$func_matches" ]]; then
        log_info "Found top-level test functions:"
        echo "$func_matches" | head -5
    fi

    if [[ -n "$suite_matches" ]]; then
        log_info "Found suite registrations:"
        echo "$suite_matches" | head -5
    fi

    if [[ -n "$method_matches" ]]; then
        log_info "Found suite test methods:"
        echo "$method_matches" | head -10
    fi
}

# Build the go test run pattern
build_test_pattern() {
    local pattern="$1"

    # Check if it's a top-level test function
    local test_func
    test_func=$(git grep -h "^func Test${pattern}" -- 'test/kubernetes/e2e/tests/*_test.go' 2>/dev/null | head -1 || true)

    if [[ -n "$test_func" ]]; then
        # Extract the test function name
        local func_name
        func_name=$(echo "$test_func" | sed -E 's/func (Test[^(]*).*/\1/')
        log_info "Running top-level test: ${func_name}"
        echo "^${func_name}$"
        return
    fi

    # Check if it's a test method within a suite (e.g., TestCookieSessionPersistence)
    if [[ "$pattern" == Test* ]]; then
        # Search for method with exact or partial match
        local method_match
        method_match=$(git grep "func (.*) ${pattern}[^(]*\(\)" -- 'test/kubernetes/e2e/features' 2>/dev/null | head -1 || true)

        if [[ -n "$method_match" ]]; then
            # Extract the actual full method name (strip line numbers and file path)
            local method_name
            method_name=$(echo "$method_match" | sed -E 's/^[^:]*:[0-9]*:func .* (Test[^(]*)\(\).*/\1/')

            # Find which file contains this method
            local method_file
            method_file=$(git grep -l "func (.*) ${method_name}\(\)" -- 'test/kubernetes/e2e/features' 2>/dev/null | head -1 || true)

            if [[ -n "$method_file" ]]; then
                # Found a test method, now find which suite it belongs to by looking at the package
                local suite_pkg
                suite_pkg=$(dirname "$method_file" | xargs basename)

                # Find the suite registration for this package
                local suite_line
                suite_line=$(git grep "Register(\"[^\"]*\", ${suite_pkg}\\.NewTestingSuite)" -- 'test/kubernetes/e2e/tests/*.go' 2>/dev/null | head -1 || true)

                if [[ -n "$suite_line" ]]; then
                    local suite_name
                    suite_name=$(echo "$suite_line" | sed -E 's/.*Register\("([^"]*)".*/\1/')

                    local file
                    file=$(echo "$suite_line" | cut -d: -f1)

                    local parent_test
                    parent_test=$(git grep "func Test.*\(t \*testing\.T\)" "$file" | cut -d: -f3- | head -1 | sed -E 's/func (Test[^(]*).*/\1/')

                    if [[ -z "$parent_test" ]]; then
                        local dir=$(dirname "$file")
                        local base=$(basename "$file" .go)
                        if [[ "$base" == *_tests ]]; then
                            local alt_file="${dir}/${base%s}.go"
                            if [[ -f "$alt_file" ]]; then
                                parent_test=$(git grep "func Test.*\(t \*testing\.T\)" "$alt_file" | cut -d: -f3- | head -1 | sed -E 's/func (Test[^(]*).*/\1/')
                            fi
                        fi
                    fi

                    if [[ -n "$parent_test" ]]; then
                        log_info "Running suite test: ${parent_test} -> ${suite_name} -> ${method_name}"
                        echo "^${parent_test}$/${suite_name}/${method_name}$"
                        return
                    fi
                fi
            fi
        fi
    fi

    # Check if it's a suite
    local suite_line
    suite_line=$(git grep "Register(\"${pattern}\"" -- 'test/kubernetes/e2e/tests/*.go' 2>/dev/null | head -1 || true)

    if [[ -z "$suite_line" ]]; then
        # Try partial match
        suite_line=$(git grep "Register(\".*${pattern}.*\"" -- 'test/kubernetes/e2e/tests/*.go' 2>/dev/null | head -1 || true)
    fi

    if [[ -n "$suite_line" ]]; then
        # Extract the suite name from Register("SuiteName", ...)
        local suite_name
        suite_name=$(echo "$suite_line" | sed -E 's/.*Register\("([^"]*)".*/\1/')

        # Find the parent test function
        local file
        file=$(echo "$suite_line" | cut -d: -f1)

        # Find which test function calls this suite runner
        local parent_test
        parent_test=$(git grep "func Test.*\(t \*testing\.T\)" "$file" | cut -d: -f3- | head -1 | sed -E 's/func (Test[^(]*).*/\1/')

        # If not found in the same file, try related files (e.g., kgateway_test.go for kgateway_tests.go)
        if [[ -z "$parent_test" ]]; then
            local dir=$(dirname "$file")
            local base=$(basename "$file" .go)
            # Try removing 's' from 'tests' suffix (kgateway_tests.go -> kgateway_test.go)
            if [[ "$base" == *_tests ]]; then
                local alt_file="${dir}/${base%s}.go"
                if [[ -f "$alt_file" ]]; then
                    parent_test=$(git grep "func Test.*\(t \*testing\.T\)" "$alt_file" | cut -d: -f3- | head -1 | sed -E 's/func (Test[^(]*).*/\1/')
                fi
            fi
        fi

        if [[ -z "$parent_test" ]]; then
            log_error "Could not find parent test function for suite: ${suite_name}"
            exit 1
        fi

        # Check if the pattern might be a specific test method (starts with Test but not the suite name)
        if [[ "$pattern" != "$suite_name" && "$pattern" == Test* ]]; then
            # The pattern looks like a test method name
            local method_match
            method_match=$(git grep -h "func (.*) ${pattern}\(\)" -- 'test/kubernetes/e2e/features' 2>/dev/null | head -1 || true)

            if [[ -n "$method_match" ]]; then
                # Running a specific test method within a suite
                local method_name
                method_name=$(echo "$method_match" | sed -E 's/func .* ([^(]*)\(\).*/\1/')
                log_info "Running suite test: ${parent_test} -> ${suite_name} -> ${method_name}"
                echo "^${parent_test}$/${suite_name}/${method_name}$"
                return
            fi
        fi

        # Running entire suite
        log_info "Running entire suite: ${parent_test} -> ${suite_name}"
        echo "^${parent_test}$/${suite_name}$"
        return
    fi

    # Fallback: just use the pattern as-is
    log_warn "Using pattern as-is: ${pattern}"
    echo "${pattern}"
}

# List available tests
list_tests() {
    log_info "Available test suites:"
    git grep -h 'Register("' -- 'test/kubernetes/e2e/tests/*.go' | \
        sed -E 's/.*Register\("([^"]*)".*/  - \1/' | sort
    echo ""
    log_info "Available top-level tests:"
    git grep -h '^func Test.*\(t \*testing\.T\)' -- 'test/kubernetes/e2e/tests/*_test.go' | \
        sed -E 's/func (Test[^(]*).*/  - \1/' | sort
}

# Main script
main() {
    local rebuild_cluster=false
    local dry_run=false
    local test_pattern=""

    # Parse arguments
    while [[ "$#" -gt 0 ]]; do
        case "$1" in
            --list|-l)
                list_tests
                exit 0
                ;;
            --rebuild|-r)
                rebuild_cluster=true
                shift
                ;;
            --dry-run|-n)
                dry_run=true
                shift
                ;;
            *)
                test_pattern="$1"
                shift
                ;;
        esac
    done

    if [[ -z "$test_pattern" ]]; then
        log_error "Usage: $0 [OPTIONS] TEST_PATTERN"
        echo ""
        echo "Examples:"
        echo "  $0 SessionPersistence"
        echo "  $0 TestCookieSessionPersistence"
        echo "  $0 TestKgateway"
        echo "  PERSIST_INSTALL=true $0 SessionPersistence"
        echo "  $0 --rebuild SessionPersistence"
        echo "  $0 -n TestCookieSessionPersistence"
        echo ""
        echo "Options:"
        echo "  --list, -l      List all available test suites and top-level tests"
        echo "  --rebuild, -r   Delete the kind cluster, rebuild images, and create a fresh cluster"
        echo "  --dry-run, -n   Print the test command that would be run without executing it"
        exit 1
    fi

    # Find test cases
    find_test_cases "$test_pattern"
    echo ""

    # Build the test run pattern
    local run_pattern
    run_pattern=$(build_test_pattern "$test_pattern")

    # Determine test package
    local test_pkg="${TEST_PKG:-./test/kubernetes/e2e/tests}"

    # Skip setup if in dry-run mode
    if [[ "$dry_run" == "true" ]]; then
        log_info "Dry-run mode: skipping setup and cluster operations"
        echo ""
    # Handle cluster rebuild
    elif [[ "$rebuild_cluster" == "true" ]]; then
        log_info "Rebuild flag set: deleting and recreating kind cluster"
        if kind_cluster_exists; then
            log_info "Deleting existing kind cluster '${CLUSTER_NAME}'..."
            ${KIND} delete cluster --name "${CLUSTER_NAME}"
            echo ""
        else
            log_info "Kind cluster '${CLUSTER_NAME}' does not exist"
        fi

        log_info "Running 'make setup' to rebuild images and create fresh cluster..."
        make setup
        echo ""
    else
        # Check for conflicting Helm releases before proceeding
        # Returns 1 if cleanup was performed, 0 otherwise
        check_and_cleanup_helm_conflicts
        local cleanup_performed=$?

        # Check if we should run setup
        local should_setup=true

        if is_truthy PERSIST_INSTALL; then
            log_info "PERSIST_INSTALL is set"
            if kind_cluster_exists; then
                if [[ $cleanup_performed -eq 1 ]]; then
                    log_info "Kind cluster '${CLUSTER_NAME}' exists, but cleanup was performed, so running setup"
                    should_setup=true
                else
                    log_info "Kind cluster '${CLUSTER_NAME}' already exists, skipping setup"
                    should_setup=false
                fi
            else
                log_info "Kind cluster '${CLUSTER_NAME}' does not exist, will run setup"
            fi
        fi

        # Run setup if needed
        if [[ "$should_setup" == "true" ]]; then
            log_info "Running 'make setup'..."
            make setup
            echo ""
        fi
    fi

    # Run the test
    log_info "Running test with pattern: ${run_pattern}"
    log_info "Test package: ${test_pkg}"
    echo ""

    # Export PERSIST_INSTALL so the test code can use it
    if is_truthy PERSIST_INSTALL; then
        export PERSIST_INSTALL
    fi

    # Escape $ for make/shell
    local escaped_pattern="${run_pattern//\$/\$\$}"

    # Set default version for tests
    local test_version="${VERSION:-1.0.0-ci1}"

    # If dry-run mode, just print the command and exit
    if [[ "$dry_run" == "true" ]]; then
        echo ""
        log_info "Dry-run mode: printing command without executing"
        echo ""
        echo "make go-test \\"
        echo "    VERSION=\"${test_version}\" \\"
        echo "    GO_TEST_USER_ARGS=\"-run '$escaped_pattern'\" \\"
        echo "    TEST_PKG=\"${test_pkg}\""
        echo ""
        log_info "Environment variables:"
        if is_truthy PERSIST_INSTALL; then
            echo "  PERSIST_INSTALL=true"
        fi
        echo ""
        log_success "Dry-run completed!"
        exit 0
    fi

    # Use a temp file to capture output for checking
    local test_output_file
    test_output_file=$(mktemp)
    trap "rm -f '$test_output_file'" EXIT

    # Temporarily disable exit-on-error so we can check the test results
    set +e
    set -x
    make go-test \
        VERSION="${test_version}" \
        "GO_TEST_USER_ARGS=-run '$escaped_pattern'" \
        TEST_PKG="${test_pkg}" 2>&1 | tee "$test_output_file"
    test_exit_code=${PIPESTATUS[0]}
    set +x
    set -e

    # Check if no tests were run
    if grep -q '\[no tests to run\]' "$test_output_file"; then
        echo ""
        log_error "No tests were run! The pattern '${run_pattern}' did not match any tests."
        log_error ""
        log_error "This usually means:"
        log_error "  1. The test pattern is incorrect"
        log_error "  2. The test name was misspelled"
        log_error "  3. The test does not exist in package ${test_pkg}"
        log_error ""
        log_error "Run '$0 --list' to see available tests"
        exit 1
    fi

    # Check the test exit code
    if [[ $test_exit_code -ne 0 ]]; then
        echo ""
        log_error "Test execution failed with exit code $test_exit_code"
        exit $test_exit_code
    fi

    echo ""
    log_success "Test execution completed!"
}

main "$@"
