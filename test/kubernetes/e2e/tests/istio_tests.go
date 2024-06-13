package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
)

func IstioTests() TestRunner {
	istioTests := new(UnorderedTests)

	istioTests.Register("PortRouting", port_routing.NewTestingSuite)
	istioTests.Register("HeadlessSvc", headless_svc.NewK8sGatewayHeadlessSvcSuite)
	istioTests.Register("IstioIntegration", istio.NewTestingSuite)
	istioTests.Register("IstioGatewayParameters", deployer.NewIstioIntegrationTestingSuite)

	return istioTests
}
