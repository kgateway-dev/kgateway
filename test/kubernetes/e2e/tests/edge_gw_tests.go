package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
)

func EdgeGwTests() e2e.SuiteRunner {
	edgeGwTests := e2e.NewSuiteRunner(false)

	edgeGwTests.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)

	return edgeGwTests
}
