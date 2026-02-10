//go:build e2e

package transformation

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/grpcurl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	// Define the setup TestCase for common resources
	setupTestCase := base.TestCase{
		Manifests: []string{},
	}

	testCases := map[string]*base.TestCase{
		"TestGatewayWithTransformedHTTPRoute": {
			Manifests: []string{
				transformForHeadersManifest,
				transformForBodyManifest,
				gatewayAttachedTransformManifest,
			},
		},
		"TestGatewayWithTransformedGRPCRoute": {
			Manifests: []string{grpcTransformationManifest},
		},
	}

	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setupTestCase, testCases),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()
}

func (s *testingSuite) TestGatewayWithTransformedHTTPRoute() {
	// Wait for the agent gateway to be ready
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
		timeout,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
		timeout,
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
				Headers: map[string]any{
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
				Headers: map[string]any{
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
				Headers: map[string]any{
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
				Headers: map[string]any{
					"x-how-great":   "level_super",
					"from-incoming": "key_level_myinnervalue",
				},
			},
		},
	}
	for _, tc := range testCases {
		allOpts := append(tc.opts,
			curl.WithHostHeader(fmt.Sprintf("example-%s.com", tc.routeName)),
		)
		common.BaseGateway.Send(
			s.T(),
			tc.resp,
			allOpts...,
		)
	}
}

// TestGatewayWithTransformedGRPCRoute needs to use grpcurl to send a gRPC request to the gateway, and verifies that the
// response includes the expected metadata header.
func (s *testingSuite) TestGatewayWithTransformedGRPCRoute() {
	// Wait for the agent gateway to be ready
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
		timeout,
	)
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
		timeout,
	)

	// Ensure the GRPCRoute is admitted and ready.
	const grpcRouteName = "example-route"
	s.TestInstallation.Assertions.EventuallyGRPCRouteCondition(s.Ctx, grpcRouteName, namespace, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
	s.TestInstallation.Assertions.EventuallyGRPCRouteCondition(s.Ctx, grpcRouteName, namespace, gwv1.RouteConditionResolvedRefs, metav1.ConditionTrue, timeout)

	// Ensure the HTTPRoute that shares the same hostname is also admitted and ready.
	// We'll use this to assert the HTTPRoute does *not* get gRPC metadata/header transformation.
	const httpRouteName = "example-route"
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(s.Ctx, httpRouteName, namespace, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(s.Ctx, httpRouteName, namespace, gwv1.RouteConditionResolvedRefs, metav1.ConditionTrue, timeout)

	// Use grpcurl from an in-cluster client pod so we exercise the actual dataplane.
	const (
		expectedHostname        = "example.com"
		gatewayPort             = 80
		expectedResponseMetaKey = "x-grpc-response"
		expectedResponseMetaVal = "from-grpc"
	)

	stdout, stderr := s.TestInstallation.AssertionsT(s.T()).AssertEventualGrpcurlSuccess(
		s.Ctx,
		kubectl.PodExecOptions{
			Name:      "grpcurl-client",
			Namespace: namespace,
			Container: "grpcurl",
		},
		[]grpcurl.Option{
			grpcurl.WithAddress(common.BaseGateway.Address),
			grpcurl.WithPort(gatewayPort),
			grpcurl.WithAuthority(expectedHostname),
			grpcurl.WithSymbol("yages.Echo/Ping"),
			grpcurl.WithPlaintext(),
			grpcurl.WithVerbose(),
			grpcurl.WithConnectTimeout(int(timeout.Seconds())),
		},
		timeout,
	)
	combined := strings.ToLower(stdout + "\n" + stderr)
	s.Require().Contains(
		combined,
		strings.ToLower(expectedResponseMetaKey)+": "+expectedResponseMetaVal,
		"expected grpcurl verbose output to contain transformed response metadata",
	)

	// Assert the HTTPRoute response does *not* include the `x-grpc-response` header, while the GRPCRoute does.
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			NotHeaders: []string{
				"x-grpc-response",
			},
		},
		curl.WithHostHeader(expectedHostname),
	)
}
