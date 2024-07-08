package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
)

func GloomtlsEdgeGwSuiteRunner() e2e.SuiteRunner {
	gloomtlsEdgeGwSuiteRunner := e2e.NewSuiteRunner(false)

	gloomtlsEdgeGwSuiteRunner.Register("Gloomtls", headless_svc.NewEdgeGatewayHeadlessSvcSuite)

	return gloomtlsEdgeGwSuiteRunner
}
