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

var _ e2e.NewSuiteFunc = NewIstioAutoMtlsSuite

// istioMtlsTestingSuite is the entire Suite of tests for the "Istio" integration cases where auto mTLS is enabled
type istioAutoMtlsTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

func NewIstioAutoMtlsSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &istioAutoMtlsTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *istioAutoMtlsTestingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.NoError(err, "can apply "+manifest)
	}

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=gw"}, time.Minute*2)
}

func (s *istioAutoMtlsTestingSuite) AfterTest(suiteName, testName string) {
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

func (s *istioAutoMtlsTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")
	// Check that istio injection is successful and httpbin is running
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, httpbinDeployment)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, httpbinDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{LabelSelector: "app=httpbin"}, time.Minute*2)

	// We include tests with manual setup here because the cleanup is still automated via AfterTest
	s.manifests = map[string][]string{
		"TestMtlsStrictPeerAuth":     {strictPeerAuthManifest, k8sRoutingSvcManifest},
		"TestMtlsPermissivePeerAuth": {permissivePeerAuthManifest, k8sRoutingSvcManifest},
		"TestMtlsDisablePeerAuth":    {disablePeerAuthManifest, k8sRoutingBackendManifest},
	}
}

func (s *istioAutoMtlsTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, httpbinDeployment)
}

func (s *istioAutoMtlsTestingSuite) TestMtlsStrictPeerAuth() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
		},
		expectedMtlsResponse, time.Minute)
}

func (s *istioAutoMtlsTestingSuite) TestMtlsPermissivePeerAuth() {
	// With auto mtls enabled in the mesh, the response should contain the X-Forwarded-Client-Cert header even with permissive mode
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPath("headers"),
		},
		expectedMtlsResponse, time.Minute)
}

func (s *istioAutoMtlsTestingSuite) TestMtlsDisablePeerAuth() {
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
