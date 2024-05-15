package istio

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/resources"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// glooIstioTestingSuite is the entire Suite of tests for the "Istio" integration cases where auto mtls is disabled
// and Upstreams do not have sslConfig values set
type glooIstioTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

func NewGlooTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	log := contextutils.LoggerFrom(ctx)

	noMtlsGlooResourcesFile := filepath.Join(testInst.GeneratedFiles.TempDir, fmt.Sprintf("glooIstioTestingSuite-%s", getGlooGatewayEdgeResourceFilmeName(UpstreamConfigOpts{})))
	sslGlooResourcesFile := filepath.Join(testInst.GeneratedFiles.TempDir, fmt.Sprintf("glooIstioTestingSuite-%s", getGlooGatewayEdgeResourceFilmeName(UpstreamConfigOpts{SetSslConfig: true})))

	noMtlsResources := GetGlooGatewayEdgeResources(testInst.Metadata.InstallNamespace, UpstreamConfigOpts{})
	err := resources.WriteResourcesToFile(noMtlsResources, noMtlsGlooResourcesFile)
	if err != nil {
		log.Error(err, "can write resources to file")
	}

	sslResources := GetGlooGatewayEdgeResources(testInst.Metadata.InstallNamespace, UpstreamConfigOpts{SetSslConfig: true})
	err = resources.WriteResourcesToFile(sslResources, sslGlooResourcesFile)
	if err != nil {
		log.Error(err, "can write resources to file")
	}

	return &glooIstioTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		manifests: map[string][]string{
			"TestStrictPeerAuth":                  {strictPeerAuthManifest, noMtlsGlooResourcesFile},
			"TestPermissivePeerAuth":              {permissivePeerAuthManifest, sslGlooResourcesFile},
			"TestUpstreamSSLConfigStrictPeerAuth": {strictPeerAuthManifest, k8sRoutingSvcManifest},
		},
	}
}

func (s *glooIstioTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, httpbinDeployment.ObjectMeta, gomega.Equal(1))
}

func (s *glooIstioTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
}

func (s *glooIstioTestingSuite) TestStrictPeerAuth() {
	// With auto mtls disabled in the mesh, the request should fail when the strict peer auth policy is applied
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
			curl.WithPort(80),
		},
		expectedServiceUnavailableResponse)
}

func (s *glooIstioTestingSuite) TestPermissivePeerAuth() {
	// With auto mtls disabled in the mesh, the response should not contain the X-Forwarded-Client-Cert header
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
			curl.WithPort(80),
		},
		expectedPlaintextResponse)
}

func (s *glooIstioTestingSuite) TestUpstreamSSLConfigStrictPeerAuth() {
	// With auto mtls disabled in the mesh, the request should fail when the strict peer auth policy is applied
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
