//go:build ignore

package istio

import (
	"context"
	"time"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// istioTestingSuite is the entire Suite of tests for the "Istio" integration cases where auto mtls is disabled
// and Upstreams do not have sslConfig values set
type istioTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

func (s *istioTestingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.NoError(err, "can apply "+manifest)
	}

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	// Check that test resources are running. This can take a little longer for Istio tests due to the istio-proxy and sds sidecars
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=gw"}, time.Minute*2)
}

func (s *istioTestingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, manifest)
		s.NoError(err, "can delete "+manifest)
	}

	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
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
	// Check that istio injection is successful and httpbin is running
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, httpbinDeployment)
	// httpbin can take a while to start up with Istio sidecar
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, httpbinDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{LabelSelector: "app=httpbin"}, time.Minute*2)

	// We include tests with manual setup here because the cleanup is still automated via AfterTest
	s.manifests = map[string][]string{
		"TestStrictPeerAuth":     {strictPeerAuthManifest, k8sRoutingSvcManifest},
		"TestPermissivePeerAuth": {permissivePeerAuthManifest, k8sRoutingSvcManifest},
	}
}

func (s *istioTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, httpbinDeployment)
}

func (s *istioTestingSuite) TestStrictPeerAuth() {
	// With auto mtls disabled in the mesh, the request should fail when the strict peer auth policy is applied
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
		},
		expectedServiceUnavailableResponse, time.Minute)
}

func (s *istioTestingSuite) TestPermissivePeerAuth() {
	// With auto mtls disabled in the mesh, the response should not contain the X-Forwarded-Client-Cert header
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
		},
		expectedPlaintextResponse, time.Minute)
}
