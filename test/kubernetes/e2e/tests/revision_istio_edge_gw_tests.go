//go:build ignore

package tests

import (
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/features/istio"
)

func RevisionIstioEdgeGatewaySuiteRunner() e2e.SuiteRunner {
	revisionIstioSuiteRunner := e2e.NewSuiteRunner(false)

	revisionIstioSuiteRunner.Register("IstioIntegration", istio.NewGlooTestingSuite)

	return revisionIstioSuiteRunner
}
