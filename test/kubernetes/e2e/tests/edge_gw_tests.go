//go:build ignore

package tests

import (
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/admin_server"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/basicrouting"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/client_tls"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/headless_svc"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/port_routing"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/tracing"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/validation/validation_allow_warnings"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/validation/validation_reject_invalid"
)

func EdgeGwSuiteRunner() e2e.SuiteRunner {
	edgeGwSuiteRunner := e2e.NewSuiteRunner(false)

	edgeGwSuiteRunner.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)
	edgeGwSuiteRunner.Register("PortRouting", port_routing.NewEdgeGatewayApiTestingSuite)
	edgeGwSuiteRunner.Register("ValidationRejectInvalid", validation_reject_invalid.NewTestingSuite)
	edgeGwSuiteRunner.Register("ValidationAllowWarnings", validation_allow_warnings.NewTestingSuite)
	edgeGwSuiteRunner.Register("GlooAdminServer", admin_server.NewTestingSuite)
	edgeGwSuiteRunner.Register("ClientTls", client_tls.NewTestingSuite)
	edgeGwSuiteRunner.Register("Tracing", tracing.NewTestingSuite)
	edgeGwSuiteRunner.Register("BasicRouting", basicrouting.NewBasicEdgeRoutingSuite)

	return edgeGwSuiteRunner
}
