package main

import (
	"log"
	"path/filepath"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/testutils/resources"
	"github.com/solo-io/skv2/codegen/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Dev tool to generate the manifest files for the test suite for demo and docs purposes
//
//go:generate go run ./generate.go
func main() {
	log.Println("starting generate for headless svc examples")

	// use the k8s gateway api resources
	k8sApiResources := []client.Object{headless_svc.K8sGateway, headless_svc.HeadlessSvcHTTPRoute}
	k8sApiRoutingGeneratedExample := filepath.Join(util.MustGetThisDir(), "generated_example", headless_svc.K8sApiRoutingGeneratedFileName)

	err := resources.WriteResourcesToFile(k8sApiResources, k8sApiRoutingGeneratedExample)
	if err != nil {
		panic(err)
	}

	// use the Gloo Edge Gateway api resources
	exampleNs := defaults.GlooSystem
	edgeGatewayApiResources := headless_svc.GetEdgeGatewayResources(exampleNs)
	edgeGatewayApiRoutingGeneratedExample := filepath.Join(util.MustGetThisDir(), "generated_example", headless_svc.EdgeGatewayApiRoutingGeneratedFileName)
	err = resources.WriteResourcesToFile(edgeGatewayApiResources, edgeGatewayApiRoutingGeneratedExample)
	if err != nil {
		panic(err)
	}

	log.Println("finished generate for headless svc examples")
}
