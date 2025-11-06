//go:build e2e

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/istio"
)

func AutomtlsIstioSuiteRunner() e2e.SuiteRunner {
	automtlsIstioSuiteRunner := e2e.NewSuiteRunner(false)

	automtlsIstioSuiteRunner.Register("IstioIntegrationAutoMtls", istio.NewIstioAutoMtlsSuite)
	automtlsIstioSuiteRunner.Register("IstioIntegrationAutoMtlsDisabled", istio.NewIstioCustomMtlsSuite)

	return automtlsIstioSuiteRunner
}
