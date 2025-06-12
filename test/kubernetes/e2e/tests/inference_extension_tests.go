package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/inferenceextension"
)

func InferenceExtensionSuiteRunner() e2e.SuiteRunner {
	infExtSuiteRunner := e2e.NewSuiteRunner(false)

	infExtSuiteRunner.Register("InferenceExtension", inferenceextension.NewTestingSuite)
	return infExtSuiteRunner
}
