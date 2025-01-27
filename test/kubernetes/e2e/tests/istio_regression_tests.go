package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
)

func IstioRegressionSuiteRunner() e2e.SuiteRunner {
	istioEdgeApiSuiteRunner := e2e.NewSuiteRunner(false)

	istioEdgeApiSuiteRunner.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)
	istioEdgeApiSuiteRunner.Register("PortRouting", port_routing.NewEdgeGatewayApiTestingSuite)
	istioEdgeApiSuiteRunner.Register("IstioIntegration", istio.NewGlooTestingSuite)

	return istioEdgeApiSuiteRunner
}
