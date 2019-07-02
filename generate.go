package main

import (
	"github.com/solo-io/gloo/pkg/version"

	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/code-generator/cmd"
	"github.com/solo-io/solo-kit/pkg/code-generator/docgen/options"
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
	if err := cmd.Generate(cmd.GenerateOptions{RelativeRoot: "projects", CompileProtos: true, GenDocs: &docsOpts, SkipGenMocks: true}); err != nil {
		log.Fatalf("generate failed!: %v", err)
	}
}
