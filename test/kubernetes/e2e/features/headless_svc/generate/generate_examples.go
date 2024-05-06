package main

import (
	"path/filepath"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/utils"
	"github.com/solo-io/skv2/codegen/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Dev tool to generate the manifest files for the test suite for demo and docs purposes
func main() {
	// use the k8s gateway api resources
	k8sApiResources := []client.Object{headless_svc.K8sGateway, headless_svc.HeadlessSvcHTTPRoute}
	k8sApiRoutingGeneratedExample := filepath.Join(util.MustGetThisDir(), "testdata", headless_svc.K8sApiRoutingGeneratedFileName)

	err := utils.WriteResourcesToFile(k8sApiResources, k8sApiRoutingGeneratedExample)
	if err != nil {
		panic(err)
	}

	// use the Gloo gateway api resources
	exampleNs := defaults.GlooSystem
	glooGatewayApiResources := headless_svc.GetGlooGatewayEdgeResources(exampleNs)
	glooGatewayApiRoutingGeneratedExample := filepath.Join(util.MustGetThisDir(), "testdata", headless_svc.GlooGatewayApiRoutingGeneratedFileName)
	err = utils.WriteResourcesToFile(glooGatewayApiResources, glooGatewayApiRoutingGeneratedExample)
	if err != nil {
		panic(err)
	}

}
