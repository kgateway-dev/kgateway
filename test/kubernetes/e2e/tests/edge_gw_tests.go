package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
)

func EdgeGwSuiteRunner() e2e.SuiteRunner {
	edgeGwSuiteRunner := e2e.NewSuiteRunner(false)

	edgeGwSuiteRunner.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)

	return edgeGwSuiteRunner
}
