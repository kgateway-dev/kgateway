package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/accesslog"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/auto_host_rewrite"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/backendconfigpolicy"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/backends"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/backendtls"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/basicrouting"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/cors"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/csrf"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/deployer"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/dfp"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/directresponse"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/extauth"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/extproc"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/header_modifiers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/http_listener_policy"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/lambda"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/leaderelection"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/loadtesting"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/path_matching"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/policyselector"
	global_rate_limit "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/rate_limit/global"
	local_rate_limit "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/rate_limit/local"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/rbac"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/route_delegation"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/services/grpcroute"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/services/httproute"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/services/tcproute"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/services/tlsroute"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/session_persistence"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/timeoutretry"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/tracing"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/transformation"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/admin_server"
)

func KubeGatewaySuiteRunner() e2e.SuiteRunner {
	kubeGatewaySuiteRunner := e2e.NewSuiteRunner(false)
	kubeGatewaySuiteRunner.Register("ExtAuth", extauth.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("AccessLog", accesslog.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Backends", backends.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("BackendTLSPolicies", backendtls.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("BasicRouting", basicrouting.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Deployer", deployer.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("DynamicForwardProxy", dfp.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("HTTPRouteServices", httproute.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("HttpListenerPolicy", http_listener_policy.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Lambda", lambda.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("RouteDelegation", route_delegation.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("SessionPersistence", session_persistence.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("TCPRouteServices", tcproute.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("TLSRouteServices", tlsroute.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("GRPCRouteServices", grpcroute.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Transforms", transformation.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("ExtProc", extproc.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("LocalRateLimit", local_rate_limit.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("GlobalRateLimit", global_rate_limit.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("PolicySelector", policyselector.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Cors", cors.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("BackendConfigPolicy", backendconfigpolicy.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("CSRF", csrf.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("AutoHostRewrite", auto_host_rewrite.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Tracing", tracing.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("AttachedRoutes", loadtesting.NewAttachedRoutesSuite)
	// kubeGatewaySuiteRunner.Register("RouteProbe", loadtesting.NewRouteProbeSuite)
	// kubeGatewaySuiteRunner.Register("RouteChange", loadtesting.NewRouteChangeSuite)
	kubeGatewaySuiteRunner.Register("DirectResponse", directresponse.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("PathMatching", path_matching.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("LeaderElection", leaderelection.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("TimeoutRetry", timeoutretry.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("HeaderModifiers", header_modifiers.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("RBAC", rbac.NewTestingSuite)

	// kubeGatewaySuiteRunner.Register("GlooAdminServer", admin_server.NewTestingSuite)

	return kubeGatewaySuiteRunner
}
