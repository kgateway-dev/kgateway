package tests

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/http_listener_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/listener_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_delegation"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/services"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/upstreams"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/virtualhost_options"
	"github.com/stretchr/testify/suite"
)

func KubeGatewayTests() e2e.SuiteRunner {
	kubeGatewayTests := e2e.NewSuiteRunner(false)

	kubeGatewayTests.Register("Deployer", deployer.NewTestingSuite)
	kubeGatewayTests.Register("HttpListenerOptions", http_listener_options.NewTestingSuite)
	kubeGatewayTests.Register("ListenerOptions", listener_options.NewTestingSuite)
	kubeGatewayTests.Register("RouteOptions", route_options.NewTestingSuite)
	kubeGatewayTests.Register("VirtualHostOptions", virtualhost_options.NewTestingSuite)
	kubeGatewayTests.Register("Upstreams", upstreams.NewTestingSuite)
	kubeGatewayTests.Register("Services", services.NewTestingSuite)
	kubeGatewayTests.Register("HeadlessSvc", headless_svc.NewK8sGatewayHeadlessSvcSuite)
	kubeGatewayTests.Register("PortRouting", port_routing.NewTestingSuite)
	kubeGatewayTests.Register("RouteDelegation", route_delegation.NewTestingSuite)
	kubeGatewayTests.Register("Glooctl", newGlooctlTestingSuite)

	return kubeGatewayTests
}

// We need to define tests requiring nesting as their own suites in order to support the injection paradigm
type glooctlSuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func newGlooctlTestingSuite(ctx context.Context, testInstallation *e2e.TestInstallation) suite.TestingSuite {
	return &glooctlSuite{
		ctx:              ctx,
		testInstallation: testInstallation,
	}
}

func (s *glooctlSuite) TestCheck() {
	suite.Run(s.T(), glooctl.NewCheckSuite(s.ctx, s.testInstallation))
}

func (s *glooctlSuite) TestDebug() {
	suite.Run(s.T(), glooctl.NewDebugSuite(s.ctx, s.testInstallation))
}

func (s *glooctlSuite) TestGetProxy() {
	suite.Run(s.T(), glooctl.NewGetProxySuite(s.ctx, s.testInstallation))
}
