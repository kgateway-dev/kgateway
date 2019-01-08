package main

import (
	"os"
	"time"

	"github.com/solo-io/gloo/pkg/version"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
	check "github.com/solo-io/go-checkpoint"
)

func main() {
	start := time.Now()
	defer check.CallCheck("glooctl", version.Version, start)

	app := cmd.GlooCli(version.Version)
	if err := app.Execute(); err != nil {
		//fmt.Println(err)
		os.Exit(1)
	}
}
