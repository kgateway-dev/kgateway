package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/helm"
)

func HelmSuiteRunner() e2e.SuiteRunner {
	helmSuiteRunner := e2e.NewSuiteRunner(false)
	helmSuiteRunner.Register("Helm", helm.NewTestingSuite)
	return helmSuiteRunner
}
