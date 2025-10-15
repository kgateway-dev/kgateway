//go:build e2e

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/aiextension"
)

func AIGatewaySuiteRunner() e2e.SuiteRunner {
	aiSuiteRunner := e2e.NewSuiteRunner(false)

	aiSuiteRunner.Register("AIExtensions", aiextension.NewSuite)
	return aiSuiteRunner
}
