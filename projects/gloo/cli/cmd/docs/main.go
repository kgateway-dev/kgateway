package main

import (
	"github.com/solo-io/go-utils/clidoc"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

func main() {
	app := cmd.GlooCli(version.Version)
	clidoc.MustGenerateCliDocs(app)
}
