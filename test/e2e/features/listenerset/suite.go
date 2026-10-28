//go:build e2e

package listenerset

import (
	"context"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
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
	gw := common.NewGateway(s.Ctx, s.T(), s.TestInstallation, types.NamespacedName{
		Name:      proxyObjectMeta.Name,
		Namespace: proxyObjectMeta.Namespace,
	})

	// The policy defined on the Gateway should apply to the Gateway listeners
	gw.Send(
		s.T(),
		expectOKWithCustomHeader("policy", "gateway"),
		curl.WithPort(gwListener1Port),
		curl.WithHostHeader("example.com"),
	)

	// The policy defined on the Gateway should apply to the Gateway section it targets
	gw.Send(
		s.T(),
		expectOKWithCustomHeader("policy", "gateway-section"),
		curl.WithPort(gwListener2Port),
		curl.WithHostHeader("example.com"),
	)

	// The policy defined on the Listener Set should apply to the Listener Set listeners
	gw.Send(
		s.T(),
		expectOKWithCustomHeader("policy", "listener-set"),
		curl.WithPort(ls1Listener1Port),
		curl.WithHostHeader("listenerset.com"),
	)

	// The policy defined on the Listener Set should apply to the Listener Set section it targets
	gw.Send(
		s.T(),
		expectOKWithCustomHeader("policy", "listener-set-section"),
		curl.WithPort(ls1Listener2Port),
		curl.WithHostHeader("listenerset.com"),
	)

	// TODO: Update this when we decide if policies should not be inherited
	// The policy defined on the Gateway should apply to the Listener Set listeners
	gw.Send(
		s.T(),
		expectOKWithCustomHeader("policy", "gateway"),
		curl.WithPort(ls2Listener1Port),
		curl.WithHostHeader("listenerset-2.com"),
	)
}
