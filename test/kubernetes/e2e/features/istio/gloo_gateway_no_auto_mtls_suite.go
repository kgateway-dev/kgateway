package istio

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/resources"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ e2e.NewSuiteFunc = NewGlooTestingSuite

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

	noMtlsGlooResourcesFile := filepath.Join(testInst.GeneratedFiles.TempDir, fmt.Sprintf("glooIstioTestingSuite-%s", getGlooGatewayEdgeResourceFile(UpstreamConfigOpts{})))
	sslGlooResourcesFile := filepath.Join(testInst.GeneratedFiles.TempDir, fmt.Sprintf("glooIstioTestingSuite-%s", getGlooGatewayEdgeResourceFile(UpstreamConfigOpts{SetSslConfig: true})))

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
			"TestPermissivePeerAuth":              {permissivePeerAuthManifest, noMtlsGlooResourcesFile},
			"TestUpstreamSSLConfigStrictPeerAuth": {strictPeerAuthManifest, sslGlooResourcesFile},
		},
	}
}

func (s *glooIstioTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")
	// Check that istio injection is successful and httpbin is running
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, httpbinDeployment)
	// httpbin can take a while to start up with Istio sidecar
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, httpbinDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{LabelSelector: "app=httpbin"}, time.Minute*2)
}

func (s *glooIstioTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, httpbinDeployment)
}

func (s *glooIstioTestingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.NoError(err, "can apply "+manifest)
	}
}

func (s *glooIstioTestingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, manifest)
		s.NoError(err, "can delete "+manifest)
	}
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
		expectedServiceUnavailableResponse, time.Minute)
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
		expectedPlaintextResponse, time.Minute)
}

func (s *glooIstioTestingSuite) TestUpstreamSSLConfigStrictPeerAuth() {
	// With auto mtls disabled in the mesh, the request should succeed when Upstream is configured with sslConfig
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
			curl.WithPort(80),
		},
		expectedMtlsResponse, time.Minute)
}
