package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/validation_always_accept"
)

func ValidationAlwaysAcceptSuiteRunner() e2e.SuiteRunner {
	validationSuiteRunner := e2e.NewSuiteRunner(false)

	validationSuiteRunner.Register("ValidationAlwaysAccept", validation_always_accept.NewTestingSuite)

	return validationSuiteRunner
}
