//go:build e2e

package grpcroute

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/grpcurl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// testingSuite is the test suite for gRPC routes
type testingSuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	var cancel context.CancelFunc
	s.ctx, cancel = context.WithTimeout(context.Background(), ctxTimeout)
	testutils.Cleanup(s.T(), cancel)

	// Apply setup manifest
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest, "-n", testNamespace)
	s.Require().NoError(err, "Failed to apply setup manifest")
}

func (s *testingSuite) TearDownSuite() {
	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	// Clean up core infrastructure
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.Require().NoError(err)
}

const (
	expectedHostname = "example.com"
)

func (s *testingSuite) TestGRPCRoute() {
	// Wait for Gateway to be accepted
	s.testInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.ctx,
		gatewayName,
		testNamespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
		timeout,
	)

	// Wait for Gateway to be programmed
	s.testInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.ctx,
		gatewayName,
		testNamespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
		timeout,
	)

	// Wait for backend Deployment and Service to be ready
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, grpcEchoDeployment, grpcEchoService)

	// Wait for Deployment to be ready
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, testNamespace, metav1.ListOptions{
		LabelSelector: "app=grpc-echo",
	})

	// Wait for GRPCRoute to be accepted
	s.testInstallation.AssertionsT(s.T()).EventuallyGRPCRouteCondition(s.ctx, grpcRouteName, testNamespace, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)

	// Wait for GRPCRoute to have resolved references
	s.testInstallation.AssertionsT(s.T()).EventuallyGRPCRouteCondition(s.ctx, grpcRouteName, testNamespace, gwv1.RouteConditionResolvedRefs, metav1.ConditionTrue, timeout)

	grpcurlOptions := []grpcurl.Option{
		grpcurl.WithAddress(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		grpcurl.WithPort(gatewayPort),
		grpcurl.WithAuthority(expectedHostname),
		grpcurl.WithSymbol(fmt.Sprintf("%s/%s", grpcServiceName, grpcMethodName)),
		grpcurl.WithPlaintext(), // Assuming HTTP listener for the gateway as per typical setup
	}

	// Assert that the grpcurl command succeeds and the JSON output matches the expected response.
	stdout, stderr := s.testInstallation.AssertionsT(s.T()).AssertEventualGrpcurlJsonResponseMatches(
		s.ctx,
		s.execOpts(),
		grpcurlOptions,
		expectedGrpcResponse,
		timeout,
	)

	s.T().Logf("AssertEventualGrpcurlJsonResponseMatches stdout:\n%s", stdout)
	s.T().Logf("AssertEventualGrpcurlJsonResponseMatches stderr:\n%s", stderr)
}

// TestGRPCRouteWithACLDeny verifies that a GRPCRoute protected by a TrafficPolicy ACL
// configured with the denyResponse.headers workaround (specifying grpc-status: 7)
// properly returns PermissionDenied to the gRPC client.
//
// Background:
// When Envoy's http-acl dynamic module denies a request, Envoy's dynamic module ABI
// (envoyproxy/envoy#48410) hardcodes grpc_status: 0, which causes Envoy to send
// HTTP 200 with grpc-status: 0 and an empty body to gRPC clients, surfacing as an EOF error.
// Until the upstream Envoy fix lands, specifying "grpc-status: 7" under denyResponse.headers
// ensures Envoy populates the gRPC trailers with status 7 (PermissionDenied) instead of 0 (OK).
func (s *testingSuite) TestGRPCRouteWithACLDeny() {
	// Apply the TrafficPolicy with ACL deny and the grpc-status: 7 header workaround
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, tpAclDenyManifest, "-n", testNamespace)
	s.Require().NoError(err, "Failed to apply TrafficPolicy ACL deny manifest")

	// Ensure cleanup after the test completes
	defer func() {
		_ = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, tpAclDenyManifest)
	}()

	grpcurlOptions := []grpcurl.Option{
		grpcurl.WithAddress(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		grpcurl.WithPort(gatewayPort),
		grpcurl.WithAuthority(expectedHostname),
		grpcurl.WithSymbol(fmt.Sprintf("%s/%s", grpcServiceName, grpcMethodName)),
		grpcurl.WithPlaintext(),
	}

	// Assert that grpcurl fails with PermissionDenied (grpc-status: 7)
	stdout, stderr := s.testInstallation.AssertionsT(s.T()).AssertEventualGrpcurlError(
		s.ctx,
		s.execOpts(),
		grpcurlOptions,
		"code = PermissionDenied",
		timeout,
	)

	s.T().Logf("AssertEventualGrpcurlError stdout:\n%s", stdout)
	s.T().Logf("AssertEventualGrpcurlError stderr:\n%s", stderr)
}

func (s *testingSuite) execOpts() kubectl.PodExecOptions {
	return kubectl.PodExecOptions{
		Name:      "grpcurl-client", // Default name for the grpcurl client pod
		Namespace: testNamespace,    // This should be set by the test case
		Container: "grpcurl",        // Default container name in the grpcurl client pod
	}
}
