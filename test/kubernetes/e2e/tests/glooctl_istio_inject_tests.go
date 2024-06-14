package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func GlooctlIstioInjectSuiteRunner() e2e.SuiteRunner {
	// NOTE: Order of tests is important here because the tests are dependent on each other (e.g. the inject test must run before the istio test)
	glooctlIstioInjectSuiteRunner := e2e.NewSuiteRunner(true)

	glooctlIstioInjectSuiteRunner.Register("GlooctlIstioInject", glooctl.NewIstioInjectTestingSuite)
	glooctlIstioInjectSuiteRunner.Register("IstioIntegration", istio.NewGlooTestingSuite)
	glooctlIstioInjectSuiteRunner.Register("GlooctlIstioUninject", glooctl.NewIstioUninjectTestingSuite)
	return glooctlIstioInjectSuiteRunner
}
