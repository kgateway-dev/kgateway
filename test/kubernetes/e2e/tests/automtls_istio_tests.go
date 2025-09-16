package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/istio"
)

func AutomtlsIstioSuiteRunner() e2e.SuiteRunner {
	automtlsIstioSuiteRunner := e2e.NewSuiteRunner(false)

	// automtlsIstioSuiteRunner.Register("PortRouting", port_routing.NewK8sGatewayTestingSuite)
	// automtlsIstioSuiteRunner.Register("HeadlessSvc", headless_svc.NewK8sGatewayHeadlessSvcSuite)
	automtlsIstioSuiteRunner.Register("IstioIntegrationAutoMtls", istio.NewIstioAutoMtlsSuite)
	automtlsIstioSuiteRunner.Register("IstioIntegrationAutoMtlsDisabled", istio.NewIstioCustomMtlsSuite)

	return automtlsIstioSuiteRunner
}
