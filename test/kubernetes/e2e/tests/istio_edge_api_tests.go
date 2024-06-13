package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func IstioEdgeApiTests() TestRunner {
	istioEdgeApiTests := make(UnorderedTests)

	istioEdgeApiTests.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)
	istioEdgeApiTests.Register("IstioIntegration", istio.NewGlooTestingSuite)

	return istioEdgeApiTests
}
