package client_tls

import (
	"context"
	"time"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/gomega/matchers"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// clientTlsTestingSuite is the entire Suite of tests for the "Gloo mtls" cases
type clientTlsTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &clientTlsTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *clientTlsTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.NginxPodManifest)
	s.NoError(err, "can apply Nginx setup manifest")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can apply Curl setup manifest")

}

func (s *clientTlsTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.NginxPodManifest)
	s.NoError(err, "can delete Nginx setup manifest")
	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can delete Curl setup manifest")
}

func (s *clientTlsTestingSuite) TestRouteSecureRequestToUpstreamFailsWithoutOneWayTls() {
	ns := s.testInstallation.Metadata.InstallNamespace
	s.T().Cleanup(func() {
		// ordering here matters if strict validation enabled
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, vsTargetingUpstreamManifestFile, "-n", ns)
		s.NoError(err, "can delete vs targeting upstream manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, vsTargetingUpstream(ns))
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, nginxUpstreamManifestFile)
		s.NoError(err, "can delete nginx upstream manifest file")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, tlsSecretManifestFile)
		s.NoError(err, "can delete tls secret manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, tlsSecret)
	})

	// ordering here matters if strict validation enabled
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, tlsSecretManifestFile)
	s.NoError(err, "can apply tls secret manifest file")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, nginxUpstreamManifestFile)
	s.NoError(err, "can apply nginx upstream manifest file")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, vsTargetingUpstreamManifestFile, "-n", ns)
	s.NoError(err, "can apply vs targeting upstream manifest file")

	s.assertEventualResponse(expectedCertVerifyFailedResponse)
}

func (s *clientTlsTestingSuite) TestRouteSecureRequestToUpstream() {
	ns := s.testInstallation.Metadata.InstallNamespace
	s.T().Cleanup(func() {
		// ordering here matters if strict validation enabled
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, vsTargetingUpstreamManifestFile, "-n", ns)
		s.NoError(err, "can delete vs targeting upstream manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, vsTargetingUpstream(ns))
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, nginxOneWayUpstreamManifestFile)
		s.NoError(err, "can delete nginx upstream manifest file")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, tlsSecretManifestFile)
		s.NoError(err, "can delete tls secret manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, tlsSecret)
	})

	// ordering here matters if strict validation enabled
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, tlsSecretManifestFile)
	s.NoError(err, "can apply tls secret manifest file")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, nginxOneWayUpstreamManifestFile)
	s.NoError(err, "can apply nginx upstream manifest file")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, vsTargetingUpstreamManifestFile, "-n", ns)
	s.NoError(err, "can apply vs targeting upstream manifest file")

	s.assertEventualResponse(expectedHealthyResponse)
}

func (s *clientTlsTestingSuite) TestRouteSecureRequestToAnnotatedServiceFailsWithoutOneWayTls() {
	ns := s.testInstallation.Metadata.InstallNamespace
	s.T().Cleanup(func() {
		// ordering here matters if strict validation enabled
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, vsTargetingKubeManifestFile, "-n", ns)
		s.NoError(err, "can delete vs targeting upstream manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, vsTargetingKube(ns))
		// this is deleted in test cleanup
		// err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, annotatedNginxSvcManifestFile)
		// s.NoError(err, "can delete nginx upstream manifest file")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, tlsSecretManifestFile)
		s.NoError(err, "can delete tls secret manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, tlsSecret)
	})

	// ordering here matters if strict validation enabled
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, tlsSecretManifestFile)
	s.NoError(err, "can apply tls secret manifest file")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, annotatedNginxSvcManifestFile)
	s.NoError(err, "can apply nginx upstream manifest file")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, vsTargetingKubeManifestFile, "-n", ns)
	s.NoError(err, "can apply vs targeting upstream manifest file")

	s.assertEventualResponse(expectedCertVerifyFailedResponse)
}

func (s *clientTlsTestingSuite) TestRouteSecureRequestToAnnotatedService() {
	ns := s.testInstallation.Metadata.InstallNamespace
	s.T().Cleanup(func() {
		// ordering here matters if strict validation enabled
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, vsTargetingKubeManifestFile, "-n", ns)
		s.NoError(err, "can delete vs targeting upstream manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, vsTargetingKube(ns))
		// this is deleted in test cleanup
		// err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, annotatedNginxOneWaySvcManifestFile)
		// s.NoError(err, "can delete nginx upstream manifest file")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, tlsSecretManifestFile)
		s.NoError(err, "can delete tls secret manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, tlsSecret)
	})

	// ordering here matters if strict validation enabled
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, tlsSecretManifestFile)
	s.NoError(err, "can apply tls secret manifest file")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, annotatedNginxOneWaySvcManifestFile)
	s.NoError(err, "can apply nginx upstream manifest file")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, vsTargetingKubeManifestFile, "-n", ns)
	s.NoError(err, "can apply vs targeting upstream manifest file")

	s.assertEventualResponse(expectedHealthyResponse)
}

func (s *clientTlsTestingSuite) assertEventualResponse(matcher *matchers.HttpResponse) {

	// Make sure our proxy pod is running
	listOpts := metav1.ListOptions{
		LabelSelector: "gloo=gateway-proxy",
	}
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, s.testInstallation.Metadata.InstallNamespace, listOpts, time.Minute*2)

	// Check curl works against expected response
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService, in our case "*"
			curl.WithHostHeader("example.com"),
			curl.WithPort(80),
		},
		matcher)
}
