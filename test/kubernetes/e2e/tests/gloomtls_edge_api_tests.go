//go:build ignore

package tests

import (
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/gloomtls"
)

func GloomtlsEdgeGwSuiteRunner() e2e.SuiteRunner {
	gloomtlsEdgeGwSuiteRunner := e2e.NewSuiteRunner(false)

	gloomtlsEdgeGwSuiteRunner.Register("Gloomtls", gloomtls.NewGloomtlsEdgeGatewayApiTestingSuite)

	return gloomtlsEdgeGwSuiteRunner
}
