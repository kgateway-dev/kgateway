package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/watch_namespace_selector"
)

func WatchNamespaceSelectorSuiteRunner() e2e.SuiteRunner {
	watchNamespaceSelectorRunner := e2e.NewSuiteRunner(false)
	watchNamespaceSelectorRunner.Register("WatchNamespaceSelector", watch_namespace_selector.NewTestingSuite)
	return watchNamespaceSelectorRunner
}
