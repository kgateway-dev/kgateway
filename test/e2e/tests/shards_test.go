package tests_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

type workflowConfig struct {
	Jobs map[string]struct {
		Strategy struct {
			Matrix struct {
				Test []struct {
					ClusterName    string `json:"cluster-name"`
					GoTestRunRegex string `json:"go-test-run-regex"`
				} `json:"test"`
			} `json:"matrix"`
		} `json:"strategy"`
	} `json:"jobs"`
}

// TestAllE2ETestsInShards verifies that every E2E test function and registered
// suite in test/e2e/tests/ is covered by at least one regex in e2e.yaml.
// This prevents new tests from being added without CI coverage.
func TestAllE2ETestsInShards(t *testing.T) {
	sourceTests := discoverE2ETestPaths(t)
	shardPaths := parseE2EWorkflowPaths(t)

	var missing []string
	for _, tp := range sourceTests {
		if !isCoveredByShardPaths(tp, shardPaths) {
			missing = append(missing, tp)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("E2E tests not covered by e2e.yaml:\n  %s\n\nUpdate .github/workflows/e2e.yaml to include these tests",
			strings.Join(missing, "\n  "))
	}
}

// discoverE2ETestPaths finds all E2E test paths by parsing source files.
// It looks for top-level test functions in *_test.go files and suite runner
// calls that link to Register calls in *_tests.go files, all with the
// //go:build e2e build tag.
//
// In kgateway, suite registration is split across files:
//   - *_test.go files define Test* functions that call suite runners
//   - *_tests.go files define suite runner functions with Register calls
//
// For test functions that call a suite runner (e.g., KubeGatewaySuiteRunner().Run()),
// individual suite paths are returned (e.g., "TestKgateway/JWT").
// For test functions with no suite runner call, the top-level function
// name is returned (e.g., "TestAPIValidation").
func discoverE2ETestPaths(t *testing.T) []string {
	t.Helper()

	allGoFiles, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("failed to glob go files: %v", err)
	}

	buildTagRe := regexp.MustCompile(`(?m)^//go:build e2e\b`)
	testFuncRe := regexp.MustCompile(`func (Test\w+)\(t \*testing\.T\)`)
	// Match direct runner calls: SuiteRunnerFunc().Run( or SuiteRunnerFunc(args).Run(
	runnerCallRe := regexp.MustCompile(`(\w+)\([^)]*\)\.Run\(`)
	// Only match static string Register calls (no string concatenation).
	// Requiring "," after the closing quote ensures we capture the full
	// suite name and skip dynamic constructions like Register("Basic/"+ns, ...).
	registerRe := regexp.MustCompile(`\.Register\("([^"]+)",`)
	// Match non-method function definitions (methods start with a receiver: func (r Type))
	funcDefRe := regexp.MustCompile(`(?m)^func (\w+)\(`)

	// Read all e2e-tagged files
	fileContents := make(map[string]string)
	for _, f := range allGoFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("failed to read %s: %v", f, err)
		}
		content := string(data)
		if buildTagRe.MatchString(content) {
			fileContents[f] = content
		}
	}

	// Build map: function name -> registered suite names.
	// For each file, track which function definition precedes each Register call.
	funcSuites := make(map[string][]string)
	for _, content := range fileContents {
		currentFunc := ""
		for line := range strings.SplitSeq(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if m := funcDefRe.FindStringSubmatch(line); m != nil {
				currentFunc = m[1]
			}
			if m := registerRe.FindStringSubmatch(line); m != nil && currentFunc != "" {
				funcSuites[currentFunc] = append(funcSuites[currentFunc], m[1])
			}
		}
	}

	// Process _test.go files to discover test paths
	var paths []string
	for f, content := range fileContents {
		if !strings.HasSuffix(f, "_test.go") {
			continue
		}

		testFuncs := testFuncRe.FindAllStringSubmatch(content, -1)
		if len(testFuncs) == 0 {
			continue
		}

		if len(testFuncs) > 1 {
			names := make([]string, len(testFuncs))
			for i, m := range testFuncs {
				names[i] = m[1]
			}
			t.Fatalf("%s has multiple Test functions (%s); split into separate files so each can be tracked independently",
				f, strings.Join(names, ", "))
		}

		testName := testFuncs[0][1]

		// Find suite runner calls in this file (e.g., KubeGatewaySuiteRunner().Run())
		var suites []string
		runnerCalls := runnerCallRe.FindAllStringSubmatch(content, -1)
		for _, rc := range runnerCalls {
			runnerName := rc[1]
			if s, ok := funcSuites[runnerName]; ok {
				suites = append(suites, s...)
			}
		}

		if len(suites) == 0 {
			paths = append(paths, testName)
		} else {
			for _, s := range suites {
				paths = append(paths, testName+"/"+s)
			}
		}
	}

	sort.Strings(paths)
	return paths
}

// shardPath represents a test path extracted from an e2e.yaml regex.
type shardPath struct {
	topLevel string // top-level test function name (e.g., "TestKgateway")
	suite    string // suite path if present (e.g., "JWT"), empty for whole-function coverage
}

func parseE2EWorkflowPaths(t *testing.T) []shardPath {
	t.Helper()

	data, err := os.ReadFile("../../../.github/workflows/e2e.yaml")
	if err != nil {
		t.Fatalf("failed to read e2e.yaml: %v", err)
	}

	var config workflowConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse e2e.yaml: %v", err)
	}

	var entries []shardPath
	for _, job := range config.Jobs {
		for _, shard := range job.Strategy.Matrix.Test {
			if shard.GoTestRunRegex == "" {
				continue
			}
			for alt := range strings.SplitSeq(shard.GoTestRunRegex, "|") {
				entries = append(entries, regexToShardPath(alt))
			}
		}
	}

	return entries
}

// regexToShardPath converts an e2e.yaml regex alternative like
// "^TestKgateway$$/^JWT$$" into a shardPath{topLevel: "TestKgateway", suite: "JWT"}.
// The $$ is Make escaping for a literal $ (the regex passes through the Makefile).
func regexToShardPath(regex string) shardPath {
	// Replace $$ (Make escaping) with $
	r := strings.ReplaceAll(regex, "$$", "$")

	// Split by $/^ to separate path levels
	parts := strings.Split(r, "$/^")

	topLevel := strings.TrimPrefix(parts[0], "^")
	topLevel = strings.TrimSuffix(topLevel, "$")

	if len(parts) == 1 {
		return shardPath{topLevel: topLevel}
	}

	var suiteParts []string
	for _, p := range parts[1:] {
		suiteParts = append(suiteParts, strings.TrimSuffix(p, "$"))
	}

	return shardPath{
		topLevel: topLevel,
		suite:    strings.Join(suiteParts, "/"),
	}
}

// isCoveredByShardPaths checks if a test path is covered by any shard entry.
// A test is covered if:
//   - Its exact path matches a shard entry (topLevel + suite)
//   - Its top-level function matches a shard entry with no suite qualifier,
//     meaning the whole function and all its subtests are covered
func isCoveredByShardPaths(testPath string, entries []shardPath) bool {
	parts := strings.SplitN(testPath, "/", 2)
	topLevel := parts[0]
	suite := ""
	if len(parts) > 1 {
		suite = parts[1]
	}

	for _, entry := range entries {
		if entry.topLevel != topLevel {
			continue
		}
		if entry.suite == "" || entry.suite == suite {
			return true
		}
	}

	return false
}
