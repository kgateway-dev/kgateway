package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
)

func AutomtlsIstioTests() TestRunner {
	automtlsIstioTests := make(UnorderedTests)

	automtlsIstioTests.Register("PortRouting", port_routing.NewTestingSuite)
	automtlsIstioTests.Register("HeadlessSvc", headless_svc.NewK8sGatewayHeadlessSvcSuite)
	automtlsIstioTests.Register("IstioIntegrationAutoMtls", istio.NewIstioAutoMtlsSuite)

	return automtlsIstioTests
}
