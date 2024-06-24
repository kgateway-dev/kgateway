package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
)

func KubeGatewayMinimalDefaultGatewayParametersSuiteRunner() e2e.SuiteRunner {
	kubeGatewayMinimalDefaultGatewayParametersSuiteRunner := e2e.NewSuiteRunner(false)

	kubeGatewayMinimalDefaultGatewayParametersSuiteRunner.Register("Deployer", deployer.NewMinimalDefaultGatewayParametersTestingSuite)

	return kubeGatewayMinimalDefaultGatewayParametersSuiteRunner
}
