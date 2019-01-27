package main

import (
	"log"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	app := cmd.GlooCli(version.Version)
	disableAutoGenTag(app)
	err := doc.GenMarkdownTree(app, "./docs/cli")
	if err != nil {
		log.Fatal(err)
	}
}

func disableAutoGenTag(c *cobra.Command) {
	c.DisableAutoGenTag = true
	for _, c := range c.Commands() {
		disableAutoGenTag(c)
	}
}
