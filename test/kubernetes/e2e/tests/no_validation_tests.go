//go:build ignore

package tests

import (
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/listener_options"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/port_routing"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/route_options"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/virtualhost_options"
)

func KubeGatewayNoValidationSuiteRunner() e2e.SuiteRunner {
	kubeGatewayNoValidationSuiteRunner := e2e.NewSuiteRunner(false)

	kubeGatewayNoValidationSuiteRunner.Register("ListenerOptions", listener_options.NewTestingSuite)
	kubeGatewayNoValidationSuiteRunner.Register("RouteOptions", route_options.NewTestingSuite)
	kubeGatewayNoValidationSuiteRunner.Register("VirtualHostOptions", virtualhost_options.NewTestingSuite)
	kubeGatewayNoValidationSuiteRunner.Register("PortRouting", port_routing.NewK8sGatewayTestingSuite)

	return kubeGatewayNoValidationSuiteRunner
}
