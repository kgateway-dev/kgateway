package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func IstioEdgeApiTests() e2e.SuiteRunner {
	istioEdgeApiTests := e2e.NewSuiteRunner(false)

	istioEdgeApiTests.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)
	istioEdgeApiTests.Register("IstioIntegration", istio.NewGlooTestingSuite)

	return istioEdgeApiTests
}
