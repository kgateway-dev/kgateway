//go:build e2e

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/agentgateway/remotejwtauth"
)

func AgentgatewaySuiteRunner() e2e.SuiteRunner {
	agentgatewaySuiteRunner := e2e.NewSuiteRunner(false)
	// agentgatewaySuiteRunner.Register("A2A", a2a.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("BasicRouting", agentgateway.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("CSRF", csrf.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("Extauth", extauth.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("LocalRateLimit", local_rate_limit.NewAgentgatewayTestingSuite)
	// agentgatewaySuiteRunner.Register("GlobalRateLimit", global_rate_limit.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("MCP", mcp.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("RBAC", rbac.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("Transformation", transformation.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("BackendTLSPolicy", backendtls.NewAgentgatewayTestingSuite)
	// agentgatewaySuiteRunner.Register("AIBackend", aibackend.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("ConfigMap", configmap.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("BasicAuth", basicauth.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("ApiKeyAuth", apikeyauth.NewTestingSuite)
	// agentgatewaySuiteRunner.Register("JwtAuth", jwtauth.NewTestingSuite)
	agentgatewaySuiteRunner.Register("RemoteJwtAuth", remotejwtauth.NewTestingSuite)

	return agentgatewaySuiteRunner
}
