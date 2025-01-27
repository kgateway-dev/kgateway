//go:build ignore

package gloomtls

import (
	"context"
	"time"

	"github.com/kgateway-dev/kgateway/projects/gateway/pkg/defaults"
	"github.com/kgateway-dev/kgateway/projects/gloo/cli/pkg/cmd/istio"
	"github.com/kgateway-dev/kgateway/test/gomega/matchers"
	testdefaults "github.com/kgateway-dev/kgateway/test/kubernetes/e2e/defaults"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewGloomtlsEdgeGatewayApiTestingSuite

// gloomtlsEdgeGatewayTestingSuite is the entire Suite of tests for the "Gloo mtls" cases
type gloomtlsEdgeGatewayTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewGloomtlsEdgeGatewayApiTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &gloomtlsEdgeGatewayTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *gloomtlsEdgeGatewayTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.NginxPodManifest)
	s.NoError(err, "can apply Nginx setup manifest")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can apply Curl setup manifest")

}

func (s *gloomtlsEdgeGatewayTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.NginxPodManifest)
	s.NoError(err, "can delete Nginx setup manifest")
	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can delete Curl setup manifest")
}

func (s *gloomtlsEdgeGatewayTestingSuite) TestRouteSecureRequestToUpstream() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, edgeRoutingResources, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.NoError(err)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, edgeRoutingResources, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err)

	// Check sds container is present
	listOpts := metav1.ListOptions{
		LabelSelector: "gloo=gateway-proxy",
	}
	matcher := gomega.And(
		matchers.PodMatches(matchers.ExpectedPod{ContainerName: istio.SDSContainerName}),
	)

	s.testInstallation.Assertions.EventuallyPodsMatches(s.ctx, s.testInstallation.Metadata.InstallNamespace, listOpts, matcher, time.Minute*2)

	// Check curl works
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("example.com"),
			curl.WithPort(80),
		},
		expectedHealthyResponse)
}
