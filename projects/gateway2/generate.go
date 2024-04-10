package main

import (
	"log"

	"github.com/solo-io/skv2/codegen"
	"github.com/solo-io/skv2/codegen/model"
	"github.com/solo-io/skv2/codegen/skv2_anyvendor"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

//go:generate go run ./generate.go
func main() {
	log.Println("starting generate for k8s gateway controller")

	anyvendorImports := skv2_anyvendor.CreateDefaultMatchOptions(
		[]string{
			"projects/gateway2/**/*.proto",
		},
	)

	skv2Cmd := codegen.Command{
		AppName:      "gloo-gateway",
		ManifestRoot: "install/helm/gloo",
		AnyVendorConfig: &skv2_anyvendor.Imports{
			Local:    anyvendorImports.Local,
			External: anyvendorImports.External,
		},
		RenderProtos: true,
		Groups: []model.Group{
			{
				Module:  "github.com/solo-io/gloo",
				ApiRoot: "projects/gateway2/pkg/api",
				GroupVersion: schema.GroupVersion{
					Group:   "gateway.gloo.solo.io",
					Version: "v1alpha1",
				},
				Resources: []model.Resource{
					{
						Kind: "GatewayParameters",
						Spec: model.Field{
							Type: model.Type{Name: "GatewayParametersSpec"},
						},
						Status: &model.Field{
							Type: model.Type{Name: "GatewayParametersStatus"},
						},
						ShortNames: []string{"gwp"},
						Stored:     true,
					},
				},
				SkipConditionalCRDLoading: true, // we want the alpha crds always rendered
				SkipTemplatedCRDManifest:  true, // do not make a copy of crds in templates dir
				RenderManifests:           true,
				RenderValidationSchemas:   true,
				RenderTypes:               true,
				RenderClients:             false,
				RenderController:          false,
				MockgenDirective:          false,
				SkipSchemaDescriptions:    true,
			},
		},
	}

	if err := skv2Cmd.Execute(); err != nil {
		log.Fatal(err)
	}

	log.Println("finished generating code for k8s gateway controller")
}
