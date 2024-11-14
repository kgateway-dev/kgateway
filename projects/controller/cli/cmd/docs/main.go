package main

import (
	"log"

	"github.com/solo-io/go-utils/clidoc"

	"github.com/solo-io/gloo/projects/controller/cli/pkg/cmd"
)

func main() {
	app := cmd.GlooCli()
	var cliGenConfig = clidoc.Config{
		OutputDir: "./docs/content/reference/cli",
	}
	err := clidoc.GenerateCliDocsWithConfig(app, cliGenConfig)
	if err != nil {
		log.Fatalf("error generating docs: %b", err)
	}

}
