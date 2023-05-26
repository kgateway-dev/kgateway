package process_skip_directives

const (
	// skipCIFile is the name of the artifact that will be published if tests should be skipped
	skipCIFile = "skip-ci.txt"
)

// ProcessChangelogDirectives checks to see if a changelog file has been added
// with any of the following fields:
//   - "skip-ci-tests"
//     (this is the only one currently, but we may add more)
//
// field set to true.
//
// Based on the directive in the changelog, it will write to a file with the set of directives that
// should be skipped. For example:
//
//	SKIP_CI_TESTS=true
//
// This file can then be pulled down by jobs in the same workflow
func ProcessChangelogDirectives() {
	// TODO - Migrate bash script to go code
}
