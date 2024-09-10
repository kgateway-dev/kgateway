package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
)

func GlooctlEdgeGwSuiteRunner() e2e.SuiteRunner {
	glooctlEdgeGwSuiteRunner := e2e.NewSuiteRunner(false)

	glooctlEdgeGwSuiteRunner.Register("Check", glooctl.NewCheckSuite)
	glooctlEdgeGwSuiteRunner.Register("CheckCrds", glooctl.NewCheckCrdsSuite)
	glooctlEdgeGwSuiteRunner.Register("Debug", glooctl.NewDebugSuite)
	glooctlEdgeGwSuiteRunner.Register("GetProxy", glooctl.NewGetProxySuite)

	return glooctlEdgeGwSuiteRunner
}
