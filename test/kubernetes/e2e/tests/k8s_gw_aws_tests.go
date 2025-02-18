//go:build ignore

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/deployer"
)

func KubeGatewayAwsSuiteRunner() e2e.SuiteRunner {
	runner := e2e.NewSuiteRunner(false)

	runner.Register("Deployer", deployer.NewTestingSuite)

	return runner
}
