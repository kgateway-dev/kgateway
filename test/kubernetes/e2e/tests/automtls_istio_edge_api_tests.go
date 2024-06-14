package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func AutomtlsIstioEdgeApiSuiteRunner() e2e.SuiteRunner {
	automtlsIstioEdgeApiSuiteRunner := e2e.NewSuiteRunner(false)

	automtlsIstioEdgeApiSuiteRunner.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)
	automtlsIstioEdgeApiSuiteRunner.Register("IstioIntegrationAutoMtls", istio.NewGlooIstioAutoMtlsSuite)

	return automtlsIstioEdgeApiSuiteRunner
}
