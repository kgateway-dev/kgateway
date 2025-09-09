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

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	// Create the GatewayClass separately because it needs to be cleaned up separately
	// at the end, after all Gateways using it are cleaned up (otherwise the GatewayClass
	// will automatically get re-created by the controller)
	s.ApplyManifests(gwcTestCase)

	s.BaseTestingSuite.BeforeTest(suiteName, testName)
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	s.BaseTestingSuite.AfterTest(suiteName, testName)

	// Delete the GatewayClass
	s.DeleteManifests(gwcTestCase)
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
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
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
