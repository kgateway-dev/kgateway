package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func RevisionIstioK8sGatewaySuiteRunner() e2e.SuiteRunner {
	istioSuiteRunner := e2e.NewSuiteRunner(false)

	istioSuiteRunner.Register("IstioIntegration", istio.NewTestingSuite)

	return istioSuiteRunner
}
