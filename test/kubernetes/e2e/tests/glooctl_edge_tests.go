package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
)

func GlooctlEdgeGwSuiteRunner() e2e.SuiteRunner {
	glooctlEdgeGwSuiteRunner := e2e.NewSuiteRunner(false)

	glooctlEdgeGwSuiteRunner.Register("GlooctlCheck", glooctl.NewCheckSuite)
	glooctlEdgeGwSuiteRunner.Register("GlooctlCheckCrds", glooctl.NewCheckCrdsSuite)
	glooctlEdgeGwSuiteRunner.Register("GlooctlCheckCrds", glooctl.NewDebugSuite)

	return glooctlEdgeGwSuiteRunner
}
