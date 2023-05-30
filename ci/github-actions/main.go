package main

import (
	"fmt"
	"os"

	process_skip_directives "github.com/solo-io/gloo/ci/github-actions/process-skip-directives"
)

func main() {
	argsWithoutProgram := os.Args[1:]
	action := argsWithoutProgram[0]

	switch action {
	case "process-skip-directives":
		process_skip_directives.ProcessChangelogDirectives()
	default:
		fmt.Printf("%s is not a supported action", action)

	}
}
