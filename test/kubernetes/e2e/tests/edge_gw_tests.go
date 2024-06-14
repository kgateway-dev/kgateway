package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
)

func EdgeGwTests() TestRunner {
	edgeGwTests := NewTestRunner(false)

	edgeGwTests.Register("HeadlessSvc", headless_svc.NewEdgeGatewayHeadlessSvcSuite)

	return edgeGwTests
}
