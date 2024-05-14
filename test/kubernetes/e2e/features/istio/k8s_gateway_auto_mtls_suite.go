package istio

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, httpbinDeployment)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, httpbinDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=httpbin",
	})
}

func (s *istioAutoMtlsTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, httpbinDeployment)
}

func (s *istioAutoMtlsTestingSuite) TestMtlsStrictPeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, strictPeerAuthManifest)
		s.NoError(err, "can delete manifest")

		// Routing with k8s svc as the destination
		err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, k8sRoutingSvcManifest)
		s.NoError(err, "can delete k8s routing manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	// Ensure that the proxy service and deployment are created
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, k8sRoutingSvcManifest)
	s.NoError(err, "can apply k8s routing manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gloo-proxy-gw",
	})

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, strictPeerAuthManifest)
	s.NoError(err, "can apply strictPeerAuthManifest")

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
		},
		expectedMtlsResponse,
	)
}

func (s *istioAutoMtlsTestingSuite) TestMtlsPermissivePeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, permissivePeerAuthManifest)
		s.NoError(err, "can delete manifest")

		// Routing with k8s svc as the destination
		err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, k8sRoutingSvcManifest)
		s.NoError(err, "can delete k8s routing manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	// Ensure that the proxy service and deployment are created
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, k8sRoutingSvcManifest)
	s.NoError(err, "can apply k8s routing manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gloo-proxy-gw",
	})

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, permissivePeerAuthManifest)
	s.NoError(err, "can apply permissivePeerAuth")

	// With auto mtls enabled in the mesh, the response should contain the X-Forwarded-Client-Cert header even with permissive mode
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
		},
		expectedMtlsResponse)
}

func (s *istioAutoMtlsTestingSuite) TestMtlsDisablePeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, disablePeerAuthManifest)
		s.NoError(err, "can delete manifest")

		// Routing with k8s svc as the destination
		err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, k8sRoutingUpstreamManifest)
		s.NoError(err, "can delete k8s routing manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	// Ensure that the proxy service and deployment are created
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, k8sRoutingUpstreamManifest)
	s.NoError(err, "can apply k8s routing manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gloo-proxy-gw",
	})

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, disablePeerAuthManifest)
	s.NoError(err, "can apply disablePeerAuthManifest")

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
		},
		expectedPlaintextResponse)
}
