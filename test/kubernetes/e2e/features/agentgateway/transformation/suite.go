package transformation

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	// Define the setup TestCase for common resources
	setupTestCase := base.TestCase{
		Manifests: []string{
			defaults.CurlPodManifest,
			simpleServiceManifest,
			gatewayManifest,
			transformForHeadersManifest,
			transformForBodyManifest,
			gatewayAttachedTransformManifest,
		},
	}

	// everything is applied during setup; there are no additional test-specific manifests
	testCases := map[string]*base.TestCase{}

	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setupTestCase, testCases),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()
}

func (s *testingSuite) TestGatewayWithTransformedRoute() {
	// Wait for the agent gateway to be ready
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)

	testCases := []struct {
		name      string
		routeName string
		opts      []curl.Option
		resp      *testmatchers.HttpResponse
	}{
		{
			name:      "basic-gateway-attached",
			routeName: "gateway-attached-transform",
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]interface{}{
					"response-gateway": "goodbye",
				},
				NotHeaders: []string{
					"x-foo-response",
				},
			},
		},
		{
			name:      "basic",
			routeName: "headers",
			opts: []curl.Option{
				curl.WithBody("hello"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]interface{}{
					"x-foo-response": "notsuper",
				},
				NotHeaders: []string{
					"response-gateway",
				},
			},
		},
		{
			name:      "conditional set by request header", // inja and the request_header function in use
			routeName: "headers",
			opts: []curl.Option{
				curl.WithBody("hello"),
				curl.WithHeader("x-add-bar", "super"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]interface{}{
					"x-foo-response": "supersupersuper",
				},
			},
		},
		{
			name:      "pull json info", // shows we parse the body as json
			routeName: "route-for-body",
			opts: []curl.Option{
				curl.WithBody(`{"mykey": {"myinnerkey": "myinnervalue"}}`),
				curl.WithHeader("X-Incoming-Stuff", "super"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]interface{}{
					"x-how-great":   "level_super",
					"from-incoming": "key_level_myinnervalue",
				},
			},
		},
	}
	for _, tc := range testCases {
		s.TestInstallation.Assertions.AssertEventualCurlResponse(
			s.Ctx,
			defaults.CurlPodExecOpt,
			append(tc.opts,
				curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
				curl.WithHostHeader(fmt.Sprintf("example-%s.com", tc.routeName)),
				curl.WithPort(8080),
			),
			tc.resp)
	}
}
