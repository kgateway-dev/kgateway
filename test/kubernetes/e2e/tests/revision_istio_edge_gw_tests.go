package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func RevisionIstioEdgeGatewaySuiteRunner() e2e.SuiteRunner {
	revisionIstioSuiteRunner := e2e.NewSuiteRunner(false)

	revisionIstioSuiteRunner.Register("IstioIntegration", istio.NewGlooTestingSuite)

	return revisionIstioSuiteRunner
}
