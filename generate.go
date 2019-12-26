package main

import (
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/code-generator/cmd"
	"github.com/solo-io/solo-kit/pkg/code-generator/docgen/options"
	"github.com/solo-io/solo-kit/pkg/protodep"
)

//go:generate go run generate.go

func main() {
	log.Printf("starting generate")

	generateOptions := cmd.GenerateOptions{
		SkipGenMocks: true,
		CustomCompileProtos: []string{
			"projects/gloo/api/grpc",
		},
		SkipGeneratedTests: true,
		// helps to cut down on time spent searching for imports, not strictly necessary
		SkipDirs: []string{
			"docs",
			"test",
			"projects/gloo/api/grpc",
		},
		RelativeRoot:  ".",
		CompileProtos: true,
		GenDocs: &cmd.DocsOptions{
			Output: options.Hugo,
			HugoOptions: &options.HugoOptions{
				DataDir: "/docs/data",
				ApiDir:  "api",
			},
		},
		ProtoDepConfig: &protodep.Config{
			Local: &protodep.Local{
				Patterns: []string{"projects/**/*.proto", protodep.SoloKitMatchPattern},
			},
			Imports: protodep.DefaultMatchOptions,
		},
	}
	if err := cmd.Generate(generateOptions); err != nil {
		log.Fatalf("generate failed!: %v", err)
	}
}
