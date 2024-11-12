package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/split_webhook"
)

func SplitWebHookGlooFailSuiteRunner() e2e.SuiteRunner {
	validationSuiteRunner := e2e.NewSuiteRunner(false)

	validationSuiteRunner.Register("GlooWebhookFail", split_webhook.NewGlooFailTestingSuite)

	return validationSuiteRunner
}
