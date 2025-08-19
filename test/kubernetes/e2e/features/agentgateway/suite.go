package agentgateway

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/suite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, base.TestCase{}, testCases),
	}
}

func (s *testingSuite) TestAgentGatewayDeployment() {
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		gatewayObjectMeta.Name,
		gatewayObjectMeta.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		gatewayObjectMeta.Name,
		gatewayObjectMeta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	// TODO: Add this once https://github.com/kgateway-dev/kgateway/issues/11929 has been resolved
	s.TestInstallation.Assertions.EventuallyGatewayListenerAttachedRoutes(
		s.Ctx,
		gatewayObjectMeta.Name,
		gatewayObjectMeta.Namespace,
		"http",
		1,
	)

	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/status/200"),
			curl.WithPort(8080),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
	)
}
