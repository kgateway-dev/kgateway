package main

import (
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/code-generator/cmd"
	"github.com/solo-io/solo-kit/pkg/code-generator/docgen/options"
	"github.com/solo-io/solo-kit/pkg/protodep"
)

//go:generate go run generate.go

func main() {
	// err := version.CheckVersions()
	// if err != nil {
	// 	log.Fatalf("generate failed!: %s", err.Error())
	// }
	log.Printf("starting generate")

	generateOptions := cmd.GenerateOptions{
		SkipGenMocks: true,
		CustomCompileProtos: []string{
			"projects/gloo/api/grpc",
		},
		SkipGeneratedTests: true,
		SkipDirs: []string{
			"docs",
			"test",
			"projects/gloo/api/grpc",
		},
		CustomImports: []string{
			"vendor/github.com/solo-io",
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
		PreRunFuncs: []cmd.RunFunc{
			protodep.PreRunProtoVendor(".",
				protodep.Options{
					MatchOptions:  protodep.DefaultMatchOptions,
					LocalMatchers: []string{"projects/**/*.proto", "projects/" + protodep.SoloKitMatchPattern},
				},
			),
		},
	}
	if err := cmd.Generate(generateOptions); err != nil {
		log.Fatalf("generate failed!: %v", err)
	}
}
