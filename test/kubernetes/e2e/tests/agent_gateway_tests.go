//go:build e2e

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway/aibackend"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway/extauth"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway/mcp"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway/rbac"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/agentgateway/transformation"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/backendtls"
	global_rate_limit "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/rate_limit/global"
	local_rate_limit "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/rate_limit/local"
)

func AgentgatewaySuiteRunner() e2e.SuiteRunner {
	agentgatewaySuiteRunner := e2e.NewSuiteRunner(false)
	// TODO: re-enable A2A tests once the image is smaller
	// agentgatewaySuiteRunner.Register("A2A", a2a.NewTestingSuite)
	agentgatewaySuiteRunner.Register("BasicRouting", agentgateway.NewTestingSuite)
	agentgatewaySuiteRunner.Register("Extauth", extauth.NewTestingSuite)
	agentgatewaySuiteRunner.Register("LocalRateLimit", local_rate_limit.NewAgentgatewayTestingSuite)
	agentgatewaySuiteRunner.Register("GlobalRateLimit", global_rate_limit.NewAgentgatewayTestingSuite)
	agentgatewaySuiteRunner.Register("MCP", mcp.NewTestingSuite)
	agentgatewaySuiteRunner.Register("RBAC", rbac.NewTestingSuite)
	agentgatewaySuiteRunner.Register("Transformation", transformation.NewTestingSuite)
	agentgatewaySuiteRunner.Register("BackendTLSPolicy", backendtls.NewAgentgatewayTestingSuite)
	agentgatewaySuiteRunner.Register("AIBackend", aibackend.NewTestingSuite)

	return agentgatewaySuiteRunner
}
