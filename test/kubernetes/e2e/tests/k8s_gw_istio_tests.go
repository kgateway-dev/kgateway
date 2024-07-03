package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
)

func IstioSuiteRunner() e2e.SuiteRunner {
	istioSuiteRunner := e2e.NewSuiteRunner(false)

	istioSuiteRunner.Register("PortRouting", port_routing.NewK8sGatewayTestingSuite)
	istioSuiteRunner.Register("HeadlessSvc", headless_svc.NewK8sGatewayHeadlessSvcSuite)
	istioSuiteRunner.Register("IstioIntegration", istio.NewTestingSuite)
	istioSuiteRunner.Register("IstioGatewayParameters", deployer.NewIstioIntegrationTestingSuite)

	return istioSuiteRunner
}
