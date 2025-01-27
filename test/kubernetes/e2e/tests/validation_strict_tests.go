package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/split_webhook"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/validation_reject_invalid"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/validation_strict_warnings"
)

// ValidationStrictSuiteRunnerAll is used to run all the validation tests, including ones that depend on the helm chart/values/helpers
// This is the function that should be used to run the validation tests in this repo
func ValidationStrictSuiteRunnerAll() e2e.SuiteRunner {
	validationSuiteRunner := ValidationStrictSuiteRunner()
	validationSuiteRunner.Register("ValidationSplitWebhook", split_webhook.NewTestingSuite)

	return validationSuiteRunner
}

// ValidationStrictSuiteRunner is used to export the validation tests that can be run when the project is imported as a helm dependency
// The "ValidationSplitWebhook" test has logic that depends on the helm chart/values/helpers
// that are not valid when the project is imported as a helm dependency
// https://github.com/kgateway-dev/kgateway/issues/10374 has been created to create a fix for this.
// If more tests are added that depend on the helm chart/values/helpers, the above issue should be resolved instead of using this approach
func ValidationStrictSuiteRunner() e2e.SuiteRunner {
	validationSuiteRunner := e2e.NewSuiteRunner(false)

	validationSuiteRunner.Register("ValidationStrictWarnings", validation_strict_warnings.NewTestingSuite)
	validationSuiteRunner.Register("ValidationRejectInvalid", validation_reject_invalid.NewTestingSuite)

	return validationSuiteRunner
}
