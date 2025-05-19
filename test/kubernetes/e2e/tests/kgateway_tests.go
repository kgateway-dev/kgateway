package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/acesslog"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/backends"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/backendtls"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/basicrouting"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/deployer"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/extauth"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/extproc"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/lambda"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/local_rate_limit"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/policyselector"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/rate_limit"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/route_delegation"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/services/grpcroute"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/services/httproute"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/services/tcproute"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/services/tlsroute"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/transformation"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/admin_server"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/crd_categories"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/directresponse"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/headless_svc"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/http_listener_options"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/listener_options"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/metrics"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/port_routing"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/route_options"
	// "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/virtualhost_options"
)

func KubeGatewaySuiteRunner() e2e.SuiteRunner {
	kubeGatewaySuiteRunner := e2e.NewSuiteRunner(false)
	kubeGatewaySuiteRunner.Register("ExtAuth", extauth.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("AccessLog", acesslog.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Backends", backends.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("BackendTLSPolicies", backendtls.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("BasicRouting", basicrouting.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Deployer", deployer.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("HTTPRouteServices", httproute.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Lambda", lambda.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("RouteDelegation", route_delegation.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("TCPRouteServices", tcproute.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("TLSRouteServices", tlsroute.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("GRPCRouteServices", grpcroute.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Transforms", transformation.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("ExtProc", extproc.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("LocalRateLimit", local_rate_limit.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("GlobalRateLimit", rate_limit.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("PolicySelector", policyselector.NewTestingSuite)

	// kubeGatewaySuiteRunner.Register("HttpListenerOptions", http_listener_options.NewTestingSuite)
	// kubeGatewaySuiteRunner.Register("ListenerOptions", listener_options.NewTestingSuite)
	// kubeGatewaySuiteRunner.Register("RouteOptions", route_options.NewTestingSuite)
	// kubeGatewaySuiteRunner.Register("VirtualHostOptions", virtualhost_options.NewTestingSuite)
	// kubeGatewaySuiteRunner.Register("HeadlessSvc", headless_svc.NewK8sGatewayHeadlessSvcSuite)
	// kubeGatewaySuiteRunner.Register("PortRouting", port_routing.NewK8sGatewayTestingSuite)
	// kubeGatewaySuiteRunner.Register("GlooAdminServer", admin_server.NewTestingSuite)
	// kubeGatewaySuiteRunner.Register("DirectResponse", directresponse.NewTestingSuite)
	// kubeGatewaySuiteRunner.Register("CRDCategories", crd_categories.NewTestingSuite)
	// kubeGatewaySuiteRunner.Register("Metrics", metrics.NewTestingSuite)

	return kubeGatewaySuiteRunner
}
