package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/validation_allow_warnings"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/validation_reject_invalid"
)

func EdgeGwSuiteRunner() e2e.SuiteRunner {
	edgeGwSuiteRunner := e2e.NewSuiteRunner(false)

	edgeGwSuiteRunner.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)
	edgeGwSuiteRunner.Register("PortRouting", port_routing.NewEdgeGatewayApiTestingSuite)
	edgeGwSuiteRunner.Register("ValidationRejectInvalid", validation_reject_invalid.NewTestingSuite)
	edgeGwSuiteRunner.Register("ValidationAllowWarnings", validation_allow_warnings.NewTestingSuite)

	return edgeGwSuiteRunner
}
