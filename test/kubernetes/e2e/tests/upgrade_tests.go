//go:build ignore

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/upgrade"
)

func UpgradeSuiteRunner() e2e.SuiteRunner {
	upgradeSuiteRunner := e2e.NewSuiteRunner(false)
	upgradeSuiteRunner.Register("Upgrade", upgrade.NewTestingSuite)
	return upgradeSuiteRunner
}
