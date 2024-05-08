package istio

import (
	"context"
	"path/filepath"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e/utils"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

// upgradeToIstioAutoMtlsTestingSuite is the entire Suite of tests for the "Istio" integration cases where an Upstream with
// sslConfig set is switched to using automtls
type upgradeToIstioAutoMtlsTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// routingManifestPath is the path to the manifest directory that contains the routing resources
	routingManifestPath string
}

func NewUpgradeToIstioAutoMtlsSuite(ctx context.Context, testInst *e2e.TestInstallation, routingManifestPath string) suite.TestingSuite {
	routingManifestFile := filepath.Join(routingManifestPath, UpstreamSslConfigEdgeApisFileName)
	return &glooIstioAutoMtlsTestingSuite{
		ctx:                 ctx,
		testInstallation:    testInst,
		routingManifestPath: routingManifestFile,
	}
}

func (s *upgradeToIstioAutoMtlsTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")

	// sslConfig defined on upstream
	resources := GetGlooGatewayEdgeResources(s.testInstallation.Metadata.InstallNamespace, false, true)
	err = utils.WriteResourcesToFile(resources, s.getEdgeGatewayRoutingManifest(true))
	s.NoError(err, "can write upstream resources to file")

	resources = GetGlooGatewayEdgeResources(s.testInstallation.Metadata.InstallNamespace, false, false)
	err = utils.WriteResourcesToFile(resources, s.getEdgeGatewayRoutingManifest(false))
	s.NoError(err, "can write upstream resources to file")
}

func (s *upgradeToIstioAutoMtlsTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
}

func (s *upgradeToIstioAutoMtlsTestingSuite) getEdgeGatewayRoutingManifest(setSSLConfig bool) string {
	if setSSLConfig {
		return filepath.Join(s.routingManifestPath, UpstreamSslConfigEdgeApisFileName)
	} else {
		// Does not have sslConfig set on upstream, relies on automtls
		return filepath.Join(s.routingManifestPath, EdgeApisRoutingResourcesFileName)
	}
}

func (s *upgradeToIstioAutoMtlsTestingSuite) TestUpgrade() {
	s.T().Cleanup(func() {
		// Clean up the final resources
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.getEdgeGatewayRoutingManifest(false))
		s.NoError(err, "can delete manifest")
	})

	// Initially use sslConfig on upstream
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.getEdgeGatewayRoutingManifest(true))
	s.NoError(err, "can apply generated routing manifest with sslConfig upstream")

	// Check sslConfig upstream is working
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
			curl.WithPort(80),
		},
		expectedMtlsResponse)

	// Switch to automtls (remove sslConfig on upstream)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.getEdgeGatewayRoutingManifest(false))
	s.NoError(err, "can apply generated routing manifest with automtls upstream")

	// Check sslConfig upstream is working
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
			curl.WithPort(80),
		},
		expectedMtlsResponse)
}

func (s *upgradeToIstioAutoMtlsTestingSuite) TestDowngrade() {
	s.T().Cleanup(func() {
		// Clean up the final resources
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.getEdgeGatewayRoutingManifest(true))
		s.NoError(err, "can delete manifest")
	})

	// Initially use automtls (remove sslConfig on upstream)
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.getEdgeGatewayRoutingManifest(false))
	s.NoError(err, "can apply generated routing manifest with automtls upstream")

	// Check sslConfig upstream is working
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
			curl.WithPort(80),
		},
		expectedMtlsResponse)

	// Switch to use sslConfig on upstream
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.getEdgeGatewayRoutingManifest(true))
	s.NoError(err, "can apply generated routing manifest with sslConfig upstream")

	// Check sslConfig upstream is working
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
			curl.WithPort(80),
		},
		expectedMtlsResponse)
}
