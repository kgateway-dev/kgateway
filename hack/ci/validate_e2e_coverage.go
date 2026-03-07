package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// E2E Test Coverage Validator
//
// This tool validates that all e2e test functions are covered by at least one
// regex pattern in the GitHub Actions workflow file (.github/workflows/e2e.yaml).
//
// Purpose:
// - Prevents newly added e2e tests from being silently skipped in CI
// - Ensures test coverage is intentional and documented
//
// Exit codes:
// - 0: All tests are covered
// - 1: Uncovered tests found or validation error

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Determine repository root
	// When run with `go run`, we need to find the repo root from current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	
	// Assume we're running from repo root or find it
	repoRoot := cwd
	if strings.HasSuffix(cwd, "hack/ci") || strings.HasSuffix(cwd, "hack\\ci") {
		repoRoot = filepath.Join(cwd, "..", "..")
	}

	// Paths
	testDir := filepath.Join(repoRoot, "test", "e2e", "tests")
	workflowFile := filepath.Join(repoRoot, ".github", "workflows", "e2e.yaml")

	// Validate paths exist
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		return fmt.Errorf("test directory not found: %s", testDir)
	}
	if _, err := os.Stat(workflowFile); os.IsNotExist(err) {
		return fmt.Errorf("workflow file not found: %s", workflowFile)
	}

	fmt.Println("🔍 Discovering e2e test functions...")
	testFunctions, err := findTestFunctions(testDir)
	if err != nil {
		return fmt.Errorf("failed to discover test functions: %w", err)
	}
	fmt.Printf("   Found %d test functions\n", len(testFunctions))

	fmt.Println("\n📋 Extracting workflow regex patterns...")
	regexPatterns, err := extractWorkflowRegexPatterns(workflowFile)
	if err != nil {
		return fmt.Errorf("failed to extract regex patterns: %w", err)
	}
	fmt.Printf("   Found %d regex patterns in workflow\n", len(regexPatterns))

	fmt.Println("\n✅ Checking test coverage...")
	uncoveredTests := []string{}

	for _, testName := range testFunctions {
		if !checkTestCoverage(testName, regexPatterns) {
			uncoveredTests = append(uncoveredTests, testName)
		}
	}

	// Report results
	if len(uncoveredTests) > 0 {
		fmt.Println("\n❌ ERROR: The following e2e tests are NOT covered by any workflow regex:\n")
		for _, testName := range uncoveredTests {
			fmt.Printf("   - %s\n", testName)
		}
		fmt.Println("\n💡 Please update the regex patterns in .github/workflows/e2e.yaml")
		fmt.Println("   to include these tests in the appropriate cluster configuration.")
		return fmt.Errorf("found %d uncovered tests", len(uncoveredTests))
	}

	fmt.Println("\n✅ SUCCESS: All e2e tests are covered by workflow regex patterns!")
	return nil
}

// findTestFunctions discovers all Go test functions in the e2e test directory.
// Returns a sorted list of test function names (e.g., "TestKgateway", "TestListenerSet")
func findTestFunctions(testDir string) ([]string, error) {
	testFunctions := make(map[string]bool)
	testPattern := regexp.MustCompile(`^func\s+(Test\w+)\s*\(\s*\w+\s+\*testing\.T`)

	// Find all *_test.go files recursively under testDir
	var files []string
	
	err := filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
	    if err != nil {
	        return err
	    }
	
	    if !info.IsDir() && strings.HasSuffix(info.Name(), "_test.go") {
	        files = append(files, path)
	    }
	
	    return nil
	})
	
	if err != nil {
	    return nil, fmt.Errorf("failed to walk test directory: %w", err)
	}

	for _, testFile := range files {
		file, err := os.Open(testFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %w", testFile, err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if matches := testPattern.FindStringSubmatch(line); matches != nil {
				testName := matches[1]
				testFunctions[testName] = true
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("error reading %s: %w", testFile, err)
		}
	}

	// Convert map to sorted slice
	result := make([]string, 0, len(testFunctions))
	for testName := range testFunctions {
		result = append(result, testName)
	}
	sort.Strings(result)

	return result, nil
}

// extractWorkflowRegexPatterns extracts all go-test-run-regex patterns from the workflow file.
// Returns a list of regex patterns used in the workflow matrix.
func extractWorkflowRegexPatterns(workflowFile string) ([]string, error) {
	file, err := os.Open(workflowFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open workflow file: %w", err)
	}
	defer file.Close()

	patterns := []string{}
	regexLinePattern := regexp.MustCompile(`^\s*go-test-run-regex:\s*['"](.+)['"]`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := regexLinePattern.FindStringSubmatch(line); matches != nil {
			pattern := matches[1]
			patterns = append(patterns, pattern)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading workflow file: %w", err)
	}

	if len(patterns) == 0 {
		return nil, fmt.Errorf("no regex patterns found in workflow file")
	}

	return patterns, nil
}

// checkTestCoverage checks if a test name is matched by any of the regex patterns.
//
// The patterns use Go's regex syntax with subtest matching like:
// ^TestKgateway$/^BasicRouting$
//
// This function checks if the test name appears in any pattern.
// It handles both patterns with and without trailing $:
// - ^TestName$ (exact match with subtests)
// - ^TestName (prefix match without subtests)
func checkTestCoverage(testName string, regexPatterns []string) bool {
	// Check for exact match pattern: ^TestName$
	exactPattern := "^" + testName + "$"
	// Check for prefix pattern: ^TestName (without $)
	prefixPattern := "^" + testName

	for _, pattern := range regexPatterns {
		// Check if ^TestName$ appears in the pattern (for tests with subtests)
		if strings.Contains(pattern, exactPattern) {
			return true
		}
		// Check if pattern ends with ^TestName (for tests without subtests)
		// This handles cases like: go-test-run-regex: '^TestMultipleInstalls'
		if strings.HasSuffix(pattern, prefixPattern) || 
		   strings.Contains(pattern, prefixPattern+"|") {
			return true
		}
	}

	return false
}


