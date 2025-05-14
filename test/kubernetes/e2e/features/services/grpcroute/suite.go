package grpcroute

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/grpcurl"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
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
	s.T().Cleanup(cancel)

	// Apply setup manifest
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest, "-n", testNamespace)
	s.Require().NoError(err, "Failed to apply setup manifest")
}

func (s *testingSuite) TearDownSuite() {
	// Clean up core infrastructure
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.Require().NoError(err)
}

const (
	expectedHostname = "example.com"
)

func (s *testingSuite) TestGRPCRoute() {
	// Wait for Gateway to be accepted
	s.testInstallation.Assertions.EventuallyGatewayCondition(
		s.ctx,
		gatewayName,
		testNamespace,
		v1.GatewayConditionAccepted,
		metav1.ConditionTrue,
		timeout,
	)

	// Wait for Gateway to be programmed
	s.testInstallation.Assertions.EventuallyGatewayCondition(
		s.ctx,
		gatewayName,
		testNamespace,
		v1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
		timeout,
	)

	// Wait for backend Deployment and Service to be ready
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, grpcEchoDeployment, grpcEchoService)

	// Wait for Deployment to be ready
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testNamespace, metav1.ListOptions{
		LabelSelector: "app=grpc-echo",
	})

	// Wait for GRPCRoute to be accepted
	s.testInstallation.Assertions.EventuallyGRPCRouteCondition(s.ctx, grpcRouteName, testNamespace, v1.RouteConditionAccepted, metav1.ConditionTrue, timeout)

	// Wait for GRPCRoute to have resolved references
	s.testInstallation.Assertions.EventuallyGRPCRouteCondition(s.ctx, grpcRouteName, testNamespace, v1.RouteConditionResolvedRefs, metav1.ConditionTrue, timeout)

	grpcurlOptions := []grpcurl.Option{
		grpcurl.WithAddress(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		grpcurl.WithPort(gatewayPort),
		grpcurl.WithAuthority(expectedHostname),
		grpcurl.WithSymbol(fmt.Sprintf("%s/%s", grpcServiceName, grpcMethodName)),
		grpcurl.WithPlaintext(), // Assuming HTTP listener for the gateway as per typical setup
	}

	// Assert that the grpcurl command succeeds and the JSON output matches the expected response.
	stdout, stderr := s.testInstallation.Assertions.AssertEventualGrpcurlJsonResponseMatches(
		s.ctx,
		s.execOpts(),
		grpcurlOptions,
		expectedGrpcResponse,
		timeout,
	)

	s.T().Logf("AssertEventualGrpcurlJsonResponseMatches stdout:\n%s", stdout)
	s.T().Logf("AssertEventualGrpcurlJsonResponseMatches stderr:\n%s", stderr)
}

func (s *testingSuite) execOpts() kubectl.PodExecOptions {
	return kubectl.PodExecOptions{
		Name:      "grpcurl-client", // Default name for the grpcurl client pod
		Namespace: testNamespace,    // This should be set by the test case
		Container: "grpcurl",        // Default container name in the grpcurl client pod
	}
}
