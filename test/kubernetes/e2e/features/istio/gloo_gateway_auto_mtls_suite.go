package istio

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/resources"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ e2e.NewSuiteFunc = NewGlooIstioAutoMtlsSuite

// glooIstioAutoMtlsTestingSuite is the entire Suite of tests for the "Istio" integration cases where auto mTLS is enabled
type glooIstioAutoMtlsTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// generated routing manifest file names
	enableAutomtlsFile              string
	disableAutomtlsFile             string
	sslConfigFile                   string
	sslConfigAndDisableAutomtlsFile string
}

func NewGlooIstioAutoMtlsSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	enableAutomtlsFile := filepath.Join(testInst.GeneratedFiles.TempDir, fmt.Sprintf("glooIstioAutoMtlsTestingSuite-%s", getGlooGatewayEdgeResourceFile(UpstreamConfigOpts{})))
	disableAutomtlsFile := filepath.Join(testInst.GeneratedFiles.TempDir, fmt.Sprintf("glooIstioAutoMtlsTestingSuite-%s", getGlooGatewayEdgeResourceFile(UpstreamConfigOpts{DisableIstioAutoMtls: true})))
	sslConfigFile := filepath.Join(testInst.GeneratedFiles.TempDir, fmt.Sprintf("glooIstioAutoMtlsTestingSuite-%s", getGlooGatewayEdgeResourceFile(UpstreamConfigOpts{SetSslConfig: true})))
	sslConfigAndDisableAutomtlsFile := filepath.Join(testInst.GeneratedFiles.TempDir, fmt.Sprintf("glooIstioAutoMtlsTestingSuite-%s", getGlooGatewayEdgeResourceFile(UpstreamConfigOpts{SetSslConfig: true, DisableIstioAutoMtls: true})))

	return &glooIstioAutoMtlsTestingSuite{
		ctx:                             ctx,
		testInstallation:                testInst,
		enableAutomtlsFile:              enableAutomtlsFile,
		disableAutomtlsFile:             disableAutomtlsFile,
		sslConfigFile:                   sslConfigFile,
		sslConfigAndDisableAutomtlsFile: sslConfigAndDisableAutomtlsFile,
	}
}

func (s *glooIstioAutoMtlsTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, httpbinDeployment.ObjectMeta, gomega.Equal(1))

	// enabled automtls on upstream
	enableAutomtlsResources := GetGlooGatewayEdgeResources(s.testInstallation.Metadata.InstallNamespace, UpstreamConfigOpts{})
	err = resources.WriteResourcesToFile(enableAutomtlsResources, s.enableAutomtlsFile)
	s.NoError(err, "can write automtls upstream resources to file")

	// disable automtls on upstream
	disableAutomtlsResources := GetGlooGatewayEdgeResources(s.testInstallation.Metadata.InstallNamespace, UpstreamConfigOpts{DisableIstioAutoMtls: true})
	err = resources.WriteResourcesToFile(disableAutomtlsResources, s.disableAutomtlsFile)
	s.NoError(err, "can write disabled automtls upstream resources to file")

	// sslConfig and automtls on upstream
	sslConfigResources := GetGlooGatewayEdgeResources(s.testInstallation.Metadata.InstallNamespace, UpstreamConfigOpts{SetSslConfig: true})
	err = resources.WriteResourcesToFile(sslConfigResources, s.sslConfigFile)
	s.NoError(err, "can write sslConfig automtls upstream resources to file")

	// sslConfig and disable automtls on upstream
	sslConfigAndDisableAutomtlsResources := GetGlooGatewayEdgeResources(s.testInstallation.Metadata.InstallNamespace, UpstreamConfigOpts{SetSslConfig: true, DisableIstioAutoMtls: true})
	err = resources.WriteResourcesToFile(sslConfigAndDisableAutomtlsResources, s.sslConfigAndDisableAutomtlsFile)
	s.NoError(err, "can write sslConfig and disable automtls upstream resources to file")
}

func (s *glooIstioAutoMtlsTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
}

func (s *glooIstioAutoMtlsTestingSuite) TestMtlsStrictPeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, strictPeerAuthManifest)
		s.NoError(err, "can delete manifest")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.enableAutomtlsFile)
		s.NoError(err, "can delete generated routing manifest")
	})

	// Ensure that the proxy service and deployment are created
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.enableAutomtlsFile)
	s.NoError(err, "can apply generated routing manifest")

	// Apply strict peer auth policy
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
		expectedMtlsResponse, time.Minute)
}

func (s *glooIstioAutoMtlsTestingSuite) TestMtlsPermissivePeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, permissivePeerAuthManifest)
		s.NoError(err, "can delete manifest")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.enableAutomtlsFile)
		s.NoError(err, "can delete generated routing manifest")
	})

	// Initially use automtls (no sslConfig on upstream)
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.enableAutomtlsFile)
	// Ensure that the proxy service and deployment are created
	s.NoError(err, "can apply generated routing manifest")

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
			curl.WithPort(80),
		},
		expectedMtlsResponse, time.Minute)

	// Apply permissive peer auth policy
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
		expectedMtlsResponse, time.Minute)
}

func (s *glooIstioAutoMtlsTestingSuite) TestMtlsDisablePeerAuth() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, disablePeerAuthManifest)
		s.NoError(err, "can delete manifest")

		// Routing with k8s svc as the destination
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.disableAutomtlsFile)
		s.NoError(err, "can delete generated routing manifest")
	})

	// Apply routing config
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.disableAutomtlsFile)
	s.NoError(err, "can apply generated routing manifest")

	// Apply disable peer auth Istio policy
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
		expectedPlaintextResponse, time.Minute)
}

func (s *glooIstioAutoMtlsTestingSuite) TestUpgrade() {
	s.T().Cleanup(func() {
		// Clean up peer auth
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, strictPeerAuthManifest)
		s.NoError(err, "can delete manifest")

		// Clean up the final resources
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.enableAutomtlsFile)
		s.NoError(err, "can delete manifest")
	})

	// Initially use sslConfig on upstream
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.sslConfigFile)
	s.NoError(err, "can apply generated routing manifest with sslConfig upstream")

	// Apply strict peer auth Istio policy to check traffic is consistently mtls
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, strictPeerAuthManifest)
	s.NoError(err, "can apply strictPeerAuthManifest")

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
		expectedMtlsResponse, time.Minute)

	// Switch to automtls (remove sslConfig on upstream)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.enableAutomtlsFile)
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
		expectedMtlsResponse, time.Minute)
}

func (s *glooIstioAutoMtlsTestingSuite) TestDowngrade() {
	s.T().Cleanup(func() {
		// Clean up peer auth
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, strictPeerAuthManifest)
		s.NoError(err, "can delete manifest")

		// Clean up the final resources
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.sslConfigFile)
		s.NoError(err, "can delete manifest")
	})

	// Initially use automtls (remove sslConfig on upstream)
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.enableAutomtlsFile)
	s.NoError(err, "can apply generated routing manifest with automtls upstream")

	// Apply strict peer auth Istio policy to check traffic is consistently mtls
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, strictPeerAuthManifest)
	s.NoError(err, "can apply strictPeerAuthManifest")

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
		expectedMtlsResponse, time.Minute)

	// Switch to use sslConfig on upstream (do not explicitly disable automtls)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.sslConfigFile)
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
		expectedMtlsResponse, time.Minute)
}

func (s *glooIstioAutoMtlsTestingSuite) TestDisableAutomtlsOverridesSSLConfig() {
	s.T().Cleanup(func() {
		// Clean up peer auth
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, disablePeerAuthManifest)
		s.NoError(err, "can delete manifest")

		// Clean up the final resources
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.sslConfigAndDisableAutomtlsFile)
		s.NoError(err, "can delete manifest")
	})

	// Uuse sslConfig on upstream with automtls disabled (sslConfig will overwrite automtls)
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.sslConfigAndDisableAutomtlsFile)
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
		expectedMtlsResponse, time.Minute)

	// Apply disable peer auth
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, disablePeerAuthManifest)
	s.NoError(err, "can apply disablePeerAuthManifest")

	// Check disable peer auth policy is working when sslConfig is set
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("/headers"),
			curl.WithPort(80),
		},
		expectedServiceUnavailableResponse, time.Minute)
}
