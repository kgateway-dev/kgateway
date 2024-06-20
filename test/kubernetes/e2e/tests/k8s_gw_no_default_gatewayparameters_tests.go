package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
)

func KubeGatewayNoDefaultGatewayParametersSuiteRunner() e2e.SuiteRunner {
	kubeGatewayNoDefaultGatewayParametersSuiteRunner := e2e.NewSuiteRunner(false)

	kubeGatewayNoDefaultGatewayParametersSuiteRunner.Register("Deployer", deployer.NewNoDefaultGatewayParametersTestingSuite)

	return kubeGatewayNoDefaultGatewayParametersSuiteRunner
}
