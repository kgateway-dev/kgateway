package istio

import (
	"context"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

// istioMtlsTestingSuite is the entire Suite of tests for the "Istio" integration cases where auto mTLS is enabled
type istioAutoMtlsTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewIstioAutoMtlsSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &istioAutoMtlsTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *istioAutoMtlsTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")
	// Check that istio injection is successful and httpbin is running
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, httpbinDeployment.ObjectMeta, gomega.Equal(1))

	// Ensure that the proxy service and deployment are created
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, k8sRoutingManifest)
	s.NoError(err, "can apply k8s routing manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
}

func (s *istioAutoMtlsTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, k8sRoutingManifest)
	s.NoError(err, "can apply k8s routing manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)

	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, httpbinDeployment)
}

func (s *istioAutoMtlsTestingSuite) TestMtlsStrictPeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, strictPeerAuthManifest)
		s.NoError(err, "can delete manifest")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, strictPeerAuthManifest)
	s.NoError(err, "can apply strictPeerAuthManifest")

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
		},
		expectedMtlsResponse,
	)
}

func (s *istioAutoMtlsTestingSuite) TestMtlsPermissivePeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, permissivePeerAuthManifest)
		s.NoError(err, "can delete manifest")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, permissivePeerAuthManifest)
	s.NoError(err, "can apply permissivePeerAuth")

	// With auto mtls enabled in the mesh, the response should contain the X-Forwarded-Client-Cert header even with permissive mode
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
		},
		expectedMtlsResponse)
}
