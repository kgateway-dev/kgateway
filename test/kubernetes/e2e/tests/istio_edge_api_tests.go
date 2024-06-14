package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func IstioEdgeApiSuiteRunner() e2e.SuiteRunner {
	istioEdgeApiSuiteRunner := e2e.NewSuiteRunner(false)

	istioEdgeApiSuiteRunner.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)
	istioEdgeApiSuiteRunner.Register("IstioIntegration", istio.NewGlooTestingSuite)

	return istioEdgeApiSuiteRunner
}
