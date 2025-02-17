//go:build ignore

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/discovery_watchlabels"
)

func DiscoveryWatchlabelsSuiteRunner() e2e.SuiteRunner {
	discoveryWatchlabelsSuiteRunner := e2e.NewSuiteRunner(false)

	discoveryWatchlabelsSuiteRunner.Register("Discovery", discovery_watchlabels.NewDiscoveryWatchlabelsSuite)

	return discoveryWatchlabelsSuiteRunner
}
