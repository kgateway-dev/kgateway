package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway/extauth"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway/rbac"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/backendtls"
)

func AgentGatewaySuiteRunner() e2e.SuiteRunner {
	agentGatewaySuiteRunner := e2e.NewSuiteRunner(false)
	agentGatewaySuiteRunner.Register("BasicRouting", agentgateway.NewTestingSuite)
	agentGatewaySuiteRunner.Register("Extauth", extauth.NewTestingSuite)
	agentGatewaySuiteRunner.Register("RBAC", rbac.NewTestingSuite)
	agentGatewaySuiteRunner.Register("BackendTLSPolicy", backendtls.NewAgentGatewayTestingSuite)

	return agentGatewaySuiteRunner
}
