//go:build e2e

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/upgrade"
)

func UpgradeSuiteRunner(fromVersion string) e2e.SuiteRunner {
	upgradeSuiteRunner := e2e.NewSuiteRunner(false)
	upgradeSuiteRunner.Register("Upgrade", upgrade.NewTestingSuite(fromVersion))
	return upgradeSuiteRunner
}
