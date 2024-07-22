package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/validation_server_disabled"
)

func ValidationServerDisabledSuiteRunner() e2e.SuiteRunner {
	validationSuiteRunner := e2e.NewSuiteRunner(false)

	validationSuiteRunner.Register("ValidationServerDisabled", validation_server_disabled.NewTestingSuite)

	return validationSuiteRunner
}
