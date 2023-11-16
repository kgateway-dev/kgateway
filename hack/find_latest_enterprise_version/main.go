package main

import (
	github_action_utils "github.com/solo-io/gloo/v2/pkg/github-action-utils"
)

func main() {
	err := github_action_utils.GetLatestEnterpriseVersion(".", "gloo", "solo-io")
	if err != nil {
		panic(err)
	}
}
