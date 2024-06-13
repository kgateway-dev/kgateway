package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func GlooctlIstioInjectTests() TestRunner {
	// NOTE: Order of tests is important here because the tests are dependent on each other (e.g. the inject test must run before the istio test)
	glooctlIstioInjectTests := make(OrderedTests, 3)

	glooctlIstioInjectTests.Register("GlooctlIstioInject", glooctl.NewIstioInjectTestingSuite)
	glooctlIstioInjectTests.Register("IstioIntegration", istio.NewGlooTestingSuite)
	glooctlIstioInjectTests.Register("GlooctlIstioUninject", glooctl.NewIstioUninjectTestingSuite)
	return glooctlIstioInjectTests
}
