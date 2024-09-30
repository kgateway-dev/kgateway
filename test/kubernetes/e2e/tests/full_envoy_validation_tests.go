package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/full_envoy_validation"
)

func FullEnvoyValidationSuiteRunner() e2e.SuiteRunner {
	validationSuiteRunner := e2e.NewSuiteRunner(false)

	validationSuiteRunner.Register("FullEnvoyValidation", full_envoy_validation.NewTestingSuite)

	return validationSuiteRunner
}
