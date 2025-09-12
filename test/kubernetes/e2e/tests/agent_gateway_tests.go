package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway/extauth"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway/rbac"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/backendtls"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/local_rate_limit"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/rate_limit"
)

func AgentGatewaySuiteRunner() e2e.SuiteRunner {
	agentGatewaySuiteRunner := e2e.NewSuiteRunner(false)
	agentGatewaySuiteRunner.Register("BasicRouting", agentgateway.NewTestingSuite)
	agentGatewaySuiteRunner.Register("Extauth", extauth.NewTestingSuite)
	agentGatewaySuiteRunner.Register("LocalRateLimit", local_rate_limit.NewTestingSuite)
	agentGatewaySuiteRunner.Register("GlobalRateLimit", rate_limit.NewAgentGatewayTestingSuite)
	agentGatewaySuiteRunner.Register("RBAC", rbac.NewTestingSuite)
	agentGatewaySuiteRunner.Register("BackendTLSPolicy", backendtls.NewAgentGatewayTestingSuite)

	return agentGatewaySuiteRunner
}
