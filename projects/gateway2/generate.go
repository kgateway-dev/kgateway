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
	//os.RemoveAll("vendor_any")
	log.Println("starting generate")

	anyvendorImports := skv2_anyvendor.CreateDefaultMatchOptions(
		[]string{
			"projects/gateway2/**/*.proto",
		},
	)
	// anyvendorImports.External["github.com/solo-io/skv2"] = []string{
	// 	"api/**/*.proto",
	// }

	skv2Cmd := codegen.Command{
		AppName:      "gateway2",
		RenderProtos: true,
		AnyVendorConfig: &skv2_anyvendor.Imports{
			Local:    anyvendorImports.Local,
			External: anyvendorImports.External,
		},
		ManifestRoot: "install/helm/gloo",
		Groups: []model.Group{
			{
				SkipConditionalCRDLoading: true, // we want the alpha crds always rendered
				SkipTemplatedCRDManifest:  true, // do not make a copy of crds in templates dir
				GroupVersion: schema.GroupVersion{
					Group:   "gateway.gloo.solo.io",
					Version: "v1alpha1",
				},
				Module:  "github.com/solo-io/gloo",
				ApiRoot: "projects/gateway2/pkg/api",
				Resources: []model.Resource{
					{
						Kind: "GatewayConfig",
						Spec: model.Field{
							Type: model.Type{Name: "GatewayConfigSpec"},
						},
						Status: &model.Field{
							Type: model.Type{Name: "GatewayConfigStatus"},
						},
						Stored: true,
					},
				},
				RenderManifests:         true,
				RenderTypes:             true,
				RenderClients:           true,
				RenderController:        true,
				MockgenDirective:        true,
				RenderValidationSchemas: true,
			},
		},
	}

	if err := skv2Cmd.Execute(); err != nil {
		log.Fatal(err)
	}

	log.Println("Finished generating code")
}
