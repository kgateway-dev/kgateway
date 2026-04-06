//go:build e2e

package listenerset

import (
	"context"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases,
			base.WithMinGwApiVersion(base.GwApiRequireListenerSets),
		),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()
}

func (s *testingSuite) TestPolicies() {
	// The policy defined on the Gateway should apply to the Gateway listeners
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(gwListener1Port),
			curl.WithHostHeader("example.com"),
		},
		expectOKWithCustomHeader("policy", "gateway"))

	// The policy defined on the Gateway should apply to the Gateway section it targets
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(gwListener2Port),
			curl.WithHostHeader("example.com"),
		},
		expectOKWithCustomHeader("policy", "gateway-section"))

	// The policy defined on the Listener Set should apply to the Listener Set listeners
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener1Port),
			curl.WithHostHeader("listenerset.com"),
		},
		expectOKWithCustomHeader("policy", "listener-set"))

	// The policy defined on the Listener Set should apply to the Listener Set section it targets
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener2Port),
			curl.WithHostHeader("listenerset.com"),
		},
		expectOKWithCustomHeader("policy", "listener-set-section"))

	// TODO: Update this when we decide if policies should not be inherited
	// The policy defined on the Gateway should apply to the Listener Set listeners
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls2Listener1Port),
			curl.WithHostHeader("listenerset-2.com"),
		},
		expectOKWithCustomHeader("policy", "gateway"))
}
