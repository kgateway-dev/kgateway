package agentgateway

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
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
	// modify the default agentgateway GatewayClass to point to the custom GatewayParameters
	err := s.TestInstallation.Actions.Kubectl().RunCommand(s.Ctx, "patch", "--type", "json",
		"gatewayclass", wellknown.DefaultAgentGatewayClassName, "-p",
		fmt.Sprintf(`[{"op": "add", "path": "/spec/parametersRef", "value": {"group":"%s", "kind":"%s", "name":"%s", "namespace":"%s"}}]`,
			v1alpha1.GroupName, wellknown.GatewayParametersGVK.Kind, gatewayParamsObjectMeta.GetName(), gatewayParamsObjectMeta.GetNamespace()))
	s.Require().NoError(err, "patching gatewayclass %s", wellknown.DefaultAgentGatewayClassName)

	s.T().Cleanup(func() {
		// revert to the original GatewayClass (by removing the parametersRef)
		err := s.TestInstallation.Actions.Kubectl().RunCommand(s.Ctx, "patch", "--type", "json",
			"gatewayclass", wellknown.DefaultAgentGatewayClassName, "-p",
			`[{"op": "remove", "path": "/spec/parametersRef"}]`)
		s.Require().NoError(err, "patching gatewayclass %s", wellknown.DefaultAgentGatewayClassName)
	})

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
