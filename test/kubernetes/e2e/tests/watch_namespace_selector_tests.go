//go:build ignore

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/watch_namespace_selector"
)

func WatchNamespaceSelectorSuiteRunner() e2e.SuiteRunner {
	watchNamespaceSelectorRunner := e2e.NewSuiteRunner(false)
	watchNamespaceSelectorRunner.Register("WatchNamespaceSelector", watch_namespace_selector.NewTestingSuite)
	return watchNamespaceSelectorRunner
}
