package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/routereplacement"
)

func RouteReplacementSuiteRunner() e2e.SuiteRunner {
	routeReplacementSuiteRunner := e2e.NewSuiteRunner(false)
	routeReplacementSuiteRunner.Register("RouteReplacement", routereplacement.NewTestingSuite)
	return routeReplacementSuiteRunner
}
