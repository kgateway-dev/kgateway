package tests

import (
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/admin_server"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/directresponse"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/http_listener_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/listener_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_delegation"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/services"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/upstreams"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/virtualhost_options"
)

func KubeGatewaySuiteRunner() e2e.SuiteRunner {
	kubeGatewaySuiteRunner := e2e.NewSuiteRunner(false)

	kubeGatewaySuiteRunner.Register("Deployer", deployer.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("HttpListenerOptions", http_listener_options.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("ListenerOptions", listener_options.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("RouteOptions", route_options.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("VirtualHostOptions", virtualhost_options.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Upstreams", upstreams.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("Services", services.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("HeadlessSvc", headless_svc.NewK8sGatewayHeadlessSvcSuite)
	kubeGatewaySuiteRunner.Register("PortRouting", port_routing.NewK8sGatewayTestingSuite)
	kubeGatewaySuiteRunner.Register("RouteDelegation", route_delegation.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("GlooAdminServer", admin_server.NewTestingSuite)
	kubeGatewaySuiteRunner.Register("DirectResponse", directresponse.NewTestingSuite)

	return kubeGatewaySuiteRunner
}
