package istio

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e/utils"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

// glooIstioAutoMtlsTestingSuite is the entire Suite of tests for the "Istio" integration cases where auto mTLS is enabled
type glooIstioAutoMtlsTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// routingManifestPath is the path to the manifest directory that contains the routing resources
	routingManifestPath string
}

func NewGlooIstioAutoMtlsSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &glooIstioAutoMtlsTestingSuite{
		ctx:                 ctx,
		testInstallation:    testInst,
		routingManifestPath: testInst.GeneratedFiles.TempDir,
	}
}

func (s *glooIstioAutoMtlsTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")

	// enabled automtls on upstream
	resources := GetGlooGatewayEdgeResources(s.testInstallation.Metadata.InstallNamespace, false, false)
	err = utils.WriteResourcesToFile(resources, s.getEdgeGatewayRoutingManifest(false))
	s.NoError(err, "can write automtls upstream resources to file")

	// disable automtls on upstream
	resources = GetGlooGatewayEdgeResources(s.testInstallation.Metadata.InstallNamespace, true, false)
	err = utils.WriteResourcesToFile(resources, s.getEdgeGatewayRoutingManifest(true))
	s.NoError(err, "can write disabled automtls upstream resources to file")
}

func (s *glooIstioAutoMtlsTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
}

func (s *glooIstioAutoMtlsTestingSuite) TestMtlsStrictPeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, strictPeerAuthManifest)
		s.NoError(err, "can delete manifest")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.getEdgeGatewayRoutingManifest(false))
		s.NoError(err, "can delete generated routing manifest")
	})

	// Ensure that the proxy service and deployment are created
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.getEdgeGatewayRoutingManifest(false))
	s.NoError(err, "can apply generated routing manifest")

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, strictPeerAuthManifest)
	s.NoError(err, "can apply strictPeerAuthManifest")

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
			curl.WithPort(80),
		},
		expectedMtlsResponse)
}

func (s *glooIstioAutoMtlsTestingSuite) TestMtlsPermissivePeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, permissivePeerAuthManifest)
		s.NoError(err, "can delete manifest")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.getEdgeGatewayRoutingManifest(false))
		s.NoError(err, "can delete generated routing manifest")
	})

	// Ensure that the proxy service and deployment are created
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.getEdgeGatewayRoutingManifest(false))
	s.NoError(err, "can apply generated routing manifest")

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, permissivePeerAuthManifest)
	s.NoError(err, "can apply permissivePeerAuth")

	// With auto mtls enabled in the mesh, the response should contain the X-Forwarded-Client-Cert header even with permissive mode
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
			curl.WithPort(80),
		},
		expectedMtlsResponse)
}

func (s *glooIstioAutoMtlsTestingSuite) TestMtlsDisablePeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, disablePeerAuthManifest)
		s.NoError(err, "can delete manifest")

		// Routing with k8s svc as the destination
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.getEdgeGatewayRoutingManifest(true))
		s.NoError(err, "can delete generated routing manifest")
	})

	// Ensure that the proxy service and deployment are created
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.getEdgeGatewayRoutingManifest(true))
	s.NoError(err, "can apply generated routing manifest")

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, disablePeerAuthManifest)
	s.NoError(err, "can apply disablePeerAuthManifest")

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
			curl.WithPort(80),
		},
		expectedPlaintextResponse)
}
