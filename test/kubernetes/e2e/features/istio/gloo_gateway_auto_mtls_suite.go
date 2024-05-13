package istio

import (
	"context"
	"path/filepath"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/resources"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (s *glooIstioAutoMtlsTestingSuite) getEdgeGatewayRoutingManifest() string {
	// TODO(npolshak): Support other upstream configurations (auto mtls disabled, sslConfig overwrite, etc.)
	return filepath.Join(s.routingManifestPath, EdgeApisRoutingResourcesFileName)
}

func (s *glooIstioAutoMtlsTestingSuite) SetupSuite() {
	gwResources := GetGlooGatewayEdgeResources(s.testInstallation.Metadata.InstallNamespace)
	err := resources.WriteResourcesToFile(gwResources, s.getEdgeGatewayRoutingManifest())
	s.NoError(err, "can write resources to file")

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")

	// Ensure that the proxy service and deployment are created
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.getEdgeGatewayRoutingManifest())
	s.NoError(err, "can apply generated routing manifest")
}

func (s *glooIstioAutoMtlsTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.getEdgeGatewayRoutingManifest())
	s.NoError(err, "can delete generated routing manifest")

	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
}

func (s *glooIstioAutoMtlsTestingSuite) TestMtlsStrictPeerAuth() {
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
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, permissivePeerAuthManifest)
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
