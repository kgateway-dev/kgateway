//go:build e2e

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/servicelabelselector"
)

func ServiceLabelSelectorSuiteRunner() e2e.SuiteRunner {
	runner := e2e.NewSuiteRunner(false)
	runner.Register("ServiceLabelSelector", servicelabelselector.NewTestingSuite)
	return runner
}
