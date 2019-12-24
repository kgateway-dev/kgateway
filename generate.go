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
		},
		CustomImports: []string{
			"vendor/github.com/solo-io",
			"vendor/github.com/solo-io/api/external",
			"vendor/github.com/envoyproxy/protoc-gen-validate",
		},
		RelativeRoot:  "",
		CompileProtos: true,
		GenDocs: &cmd.DocsOptions{
			Output: options.Hugo,
			HugoOptions: &options.HugoOptions{
				DataDir: "/docs/data",
				ApiDir:  "api",
			},
		},
		PreRunFuncs: []cmd.RunFunc{
			protodep.PreRunProtoVendor(".", protodep.DefaultMatchOptions),
		},
	}
	if err := cmd.Generate(generateOptions); err != nil {
		log.Fatalf("generate failed!: %v", err)
	}
}
