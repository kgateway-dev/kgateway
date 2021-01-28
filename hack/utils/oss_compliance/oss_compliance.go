package main

import (
	"fmt"
	"os"

	"github.com/solo-io/go-list-licenses/pkg/license"
)

func main() {
	glooPackages := []string{
		"github.com/solo-io/gloo/projects/accesslogger/cmd",
		"github.com/solo-io/gloo/projects/discovery/cmd",
		"github.com/solo-io/gloo/projects/envoyinit/cmd",
		"github.com/solo-io/gloo/projects/gateway/cmd",
		"github.com/solo-io/gloo/projects/gloo/cmd",
		"github.com/solo-io/gloo/projects/ingress/cmd",
		"github.com/solo-io/gloo/projects/hypergloo",
	}

	// dependencies for this package which are used on mac, and will not be present in linux CI
	macOnlyDependencies := []string{
		"github.com/mitchellh/go-homedir",
		"github.com/containerd/continuity",
	}

	app := license.Cli(glooPackages, macOnlyDependencies)
	if err := app.Execute(); err != nil {
		fmt.Errorf("unable to run oss compliance check: %v\n", err)
		os.Exit(1)
	}
}
