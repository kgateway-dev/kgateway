package process_skip_directives

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
)

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
	fmt.Print("RUNNING PROCESS CHANGELOG DIRECTIVES")

	cmd := exec.Command("git", "diff origin/main HEAD --name-only | grep `changelog/` | wc -l")
	bytes, err := cmd.Output()
	if err != nil {
		switch err.(type) {
		case *exec.ExitError:
			// this is just an exit code error, no worries
			// do nothing

		default: // Actual error
			log.Fatalf("Error while trying to identify number of changelog files: %v", err)
		}
	}

	numberOfChangelogFiles, err := strconv.Atoi(string(bytes))
	if err != nil {
		log.Fatalf("Error while converting string to int: %s", err.Error())
	}

	if numberOfChangelogFiles == 0 {
		log.Print("No changelog files found")
		return
	}

	if numberOfChangelogFiles > 1 {
		log.Printf("More than 1 changelog files found: %d", numberOfChangelogFiles)
	}

	log.Print("2 changelog file found")
}
