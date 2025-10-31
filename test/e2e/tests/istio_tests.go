//go:build ignore && e2e

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/deployer"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/istio"
)

func IstioSuiteRunner() e2e.SuiteRunner {
	istioSuiteRunner := e2e.NewSuiteRunner(false)

	istioSuiteRunner.Register("IstioIntegration", istio.NewTestingSuite)
	istioSuiteRunner.Register("IstioGatewayParameters", deployer.NewIstioIntegrationTestingSuite)

	return istioSuiteRunner
}
