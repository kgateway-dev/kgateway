//go:build ignore

package tests

import (
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/headless_svc"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/istio"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/port_routing"
)

func AutomtlsIstioEdgeApiSuiteRunner() e2e.SuiteRunner {
	automtlsIstioEdgeApiSuiteRunner := e2e.NewSuiteRunner(false)

	automtlsIstioEdgeApiSuiteRunner.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)
	automtlsIstioEdgeApiSuiteRunner.Register("PortRouting", port_routing.NewEdgeGatewayApiTestingSuite)
	automtlsIstioEdgeApiSuiteRunner.Register("IstioIntegrationAutoMtls", istio.NewGlooIstioAutoMtlsSuite)

	return automtlsIstioEdgeApiSuiteRunner
}
