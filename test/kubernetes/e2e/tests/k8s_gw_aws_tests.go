package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
)

func KubeGatewayAwsSuiteRunner() e2e.SuiteRunner {
	runner := e2e.NewSuiteRunner(false)

	runner.Register("Deployer", deployer.NewTestingSuite)

	return runner
}
