package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func GlooctlIstioInjectTests() e2e.SuiteRunner {
	// NOTE: Order of tests is important here because the tests are dependent on each other (e.g. the inject test must run before the istio test)
	glooctlIstioInjectTests := e2e.NewSuiteRunner(true)

	glooctlIstioInjectTests.Register("GlooctlIstioInject", glooctl.NewIstioInjectTestingSuite)
	glooctlIstioInjectTests.Register("IstioIntegration", istio.NewGlooTestingSuite)
	glooctlIstioInjectTests.Register("GlooctlIstioUninject", glooctl.NewIstioUninjectTestingSuite)
	return glooctlIstioInjectTests
}
