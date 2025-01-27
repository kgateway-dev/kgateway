//go:build ignore

package tests

import (
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/headless_svc"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/istio"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/port_routing"
)

func IstioEdgeApiSuiteRunner() e2e.SuiteRunner {
	istioEdgeApiSuiteRunner := e2e.NewSuiteRunner(false)

	istioEdgeApiSuiteRunner.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)
	istioEdgeApiSuiteRunner.Register("PortRouting", port_routing.NewEdgeGatewayApiTestingSuite)
	istioEdgeApiSuiteRunner.Register("IstioIntegration", istio.NewGlooTestingSuite)

	return istioEdgeApiSuiteRunner
}
