package main

import (
	"fmt"

	github_action_utils "github.com/solo-io/gloo/pkg/github-action-utils"
)

func main() {
	version, err := github_action_utils.GetVersion(".", "gloo", "solo-io")
	if err != nil {
		panic(err)
	}
	fmt.Print(version)
}
