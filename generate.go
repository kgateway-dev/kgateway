package main

import (
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/code-generator/cmd"
	"github.com/solo-io/solo-kit/pkg/code-generator/docgen/options"
	"github.com/solo-io/solo-kit/pkg/code-generator/schemagen"
	"github.com/solo-io/solo-kit/pkg/code-generator/sk_anyvendor"
)

//go:generate go run generate.go

func main() {
	log.Printf("starting generate")

	protoImports := sk_anyvendor.CreateDefaultMatchOptions(
		[]string{"projects/**/*.proto", sk_anyvendor.SoloKitMatchPattern},
	)
	protoImports.External["github.com/solo-io/solo-apis"] = []string{
		"api/rate-limiter/**/*.proto", // Import rate limit API
		"api/gloo-fed/fed/**/*.proto", // Import gloo fed gloo instance API
	}
	// Import gloo instance API dependencies
	protoImports.External["github.com/solo-io/skv2"] = []string{
		"api/**/**/*.proto",
	}

	generateOptions := cmd.GenerateOptions{
		SkipGenMocks: true,
		CustomCompileProtos: []string{
			"github.com/solo-io/gloo/projects/gloo/api/grpc",
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
				ApiDir:  "reference/api",
			},
		},
		ExternalImports: protoImports,
		ValidationSchemaOptions: &schemagen.ValidationSchemaOptions{
			CrdDirectory:                 "install/helm/gloo/crds",
			JsonSchemaTool:               "protoc",
			RemoveDescriptionsFromSchema: true,
			EnumAsIntOrString:            true,
			MessagesWithEmptySchema: []string{
				// These messages are recursive, and will cause codegen to enter an infinite loop
				"core.solo.io.Status",
				"ratelimit.api.solo.io.Descriptor",
				"als.options.gloo.solo.io.AndFilter",
				"als.options.gloo.solo.io.OrFilter",

				// These messages are part of our internal API, and therefore aren't required
				// Also they are quite large and can cause the Proxy CRD to become too large,
				// resulting in: https://github.com/solo-io/gloo/issues/4789
				"gloo.solo.io.HttpListener",
				"gloo.solo.io.TcpListener",
				"gloo.solo.io.HybridListener",
				"gloo.solo.io.AggregateListener",
			},
		},
	}
	if err := cmd.Generate(generateOptions); err != nil {
		log.Fatalf("generate failed!: %v", err)
	}
	log.Printf("finished generating code")
}
