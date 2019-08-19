package main

import (
	"os"

	"github.com/solo-io/gloo/pkg/version"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
	check "github.com/solo-io/go-checkpoint"
)

func main() {
	check.NewUsageClient().Start("glooctl", version.Version)
	app := cmd.GlooCli(version.Version)
	if err := app.Execute(); err != nil {
		//fmt.Println(err)
		os.Exit(1)
	}
}
