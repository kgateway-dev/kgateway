package main

import (
	"github.com/solo-io/gloo/pkg/version"

	"github.com/solo-io/solo-kit/pkg/code-generator/cmd"
	"github.com/solo-io/solo-kit/pkg/code-generator/docgen/options"
	"github.com/solo-io/solo-kit/pkg/utils/log"
)

//go:generate go run generate.go



func main() {
	err := version.CheckVersions()
	if err != nil {
		log.Fatalf("generate failed!: %v", err)
	}
	log.Printf("starting generate")
	docsOpts := cmd.DocsOptions{
		Output: options.Hugo,
	}
	if err := cmd.Run("projects", true, &docsOpts, nil, nil); err != nil {
		log.Fatalf("generate failed!: %v", err)
	}
}
