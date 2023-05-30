package main

import (
	"fmt"
	process_skip_directives "github.com/solo-io/gloo/ci/github-actions/process-skip-directives"
	"os"
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
