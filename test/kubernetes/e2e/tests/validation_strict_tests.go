package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/validation_reject_invalid"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/validation_strict_warnings"
)

func ValidationStrictSuiteRunner() e2e.SuiteRunner {
	validationSuiteRunner := e2e.NewSuiteRunner(false)

	validationSuiteRunner.Register("ValidationStrictWarnings", validation_strict_warnings.NewTestingSuite)
	validationSuiteRunner.Register("ValidationRejectInvalid", validation_reject_invalid.NewTestingSuite)

	return validationSuiteRunner
}
