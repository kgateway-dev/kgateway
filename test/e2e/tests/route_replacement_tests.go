//go:build e2e

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/routereplacement"
)

func RouteReplacementSuiteRunner() e2e.SuiteRunner {
	routeReplacementSuiteRunner := e2e.NewSuiteRunner(false)
	routeReplacementSuiteRunner.Register("RouteReplacement", routereplacement.NewTestingSuite)
	return routeReplacementSuiteRunner
}
