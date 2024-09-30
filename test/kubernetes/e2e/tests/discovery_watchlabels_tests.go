package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/discovery_watchlabels"
)

func DiscoveryWatchlabelsSuiteRunner() e2e.SuiteRunner {
	discoveryWatchlabelsSuiteRunner := e2e.NewSuiteRunner(false)

	discoveryWatchlabelsSuiteRunner.Register("Discovery", discovery_watchlabels.NewDiscoveryWatchlabelsSuite)

	return discoveryWatchlabelsSuiteRunner
}
