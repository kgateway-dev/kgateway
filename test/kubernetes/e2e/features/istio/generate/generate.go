package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
	"github.com/solo-io/gloo/test/kubernetes/testutils/resources"
	"github.com/solo-io/skv2/codegen/util"
)

const (
	// exampleNs is the namespace where the resources will be created. Change this to the namespace where you want to create the resources
	exampleNs = defaults.GlooSystem
)

// Dev tool to generate the manifest files for the test suite for demo and docs purposes
//
//go:generate go run ./generate.go
func main() {
	log.Println("starting generate for istio examples")

	// use the Gloo Edge Gateway api resources with automtls enabled
	edgeGatewayApiResources := istio.GetGlooGatewayEdgeResources(exampleNs)
	automtlsGeneratedExample := filepath.Join(util.MustGetThisDir(), "generated_example", fmt.Sprintf("automtls-enabled-%s", istio.EdgeApisRoutingResourcesFileName))
	err := resources.WriteResourcesToFile(edgeGatewayApiResources, automtlsGeneratedExample)
	if err != nil {
		panic(err)
	}

	log.Println("finished generate for istio examples")
}
