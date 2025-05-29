package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/listenerset"
)

func ListenerSetSuiteRunner() e2e.SuiteRunner {
	suiteRunner := e2e.NewSuiteRunner(false)
	suiteRunner.Register("ListenerSet", listenerset.NewTestingSuite)
	return suiteRunner
}
