package istio

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

// istioTestingSuite is the entire Suite of tests for the "Istio" integration cases where auto-mtls is disabled
// and Upstreams do not have sslConfig values set
type istioTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &istioTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *istioTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")

	// Ensure that the proxy service and deployment are created
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, k8sRoutingManifest)
	s.NoError(err, "can apply k8s routing manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
}

func (s *istioTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, k8sRoutingManifest)
	s.NoError(err, "can apply k8s routing manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)

	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
}

func (s *istioTestingSuite) TestStrictPeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, strictPeerAuthManifest)
		s.NoError(err, "can delete manifest")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, strictPeerAuthManifest)
	s.NoError(err, "can apply strictPeerAuthManifest")

	// With auto mtls disabled in the mesh, the request should fail when the strict peer auth policy is applied
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
		},
		expectedServiceUnavailableResponse)
}

func (s *istioTestingSuite) TestPermissivePeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, permissivePeerAuthManifest)
		s.NoError(err, "can delete manifest")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, permissivePeerAuthManifest)
	s.NoError(err, "can apply permissivePeerAuth")

	// With auto mtls disabled in the mesh, the response should not contain the X-Forwarded-Client-Cert header
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
		},
		expectedPlaintextResponse)
}
