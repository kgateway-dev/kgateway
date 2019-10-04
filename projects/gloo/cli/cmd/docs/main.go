package main

import (
	"github.com/solo-io/go-utils/clidoc"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

func main() {
	app := cmd.GlooCli()
	clidoc.MustGenerateCliDocs(app)
}
