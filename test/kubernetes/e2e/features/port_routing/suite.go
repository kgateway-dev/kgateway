//go:build ignore

package port_routing

import (
	"context"
	"time"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewK8sGatewayTestingSuite

// portRoutingTestingSuite is the entire Suite of tests for the "PortRouting" cases
type portRoutingTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

/*
The port routing suite sets up in the following order

SetupSuite:
 1. Create the setup apps (curl, nginx, etc.)
 2. Create k8s Gateway
 3. Proxy provisioned (k8s deployment created and checked)

Each port routing test:
 1. Attach HttpRoute with different port/targetport definition per test
 2. Remove HttpRoute, proxy still exists without any routes

TearDownSuite:
 1. Deletes the setup apps (curl, nginx, etc.)
 2. Deletes the k8s Gateway
 3. Proxy de-provisioned (k8s deployment deleted)
*/
func NewK8sGatewayTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &portRoutingTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		manifests: map[string][]string{
			"TestInvalidPortAndValidTargetport":   {svcInvalidPortAndValidTargetportManifest, invalidPortAndValidTargetportManifest},
			"TestMatchPortAndTargetport":          {svcMatchPortandTargetportManifest, matchPortandTargetportManifest},
			"TestMatchPodPortWithoutTargetport":   {svcMatchPodPortWithoutTargetportManifest, matchPodPortWithoutTargetportManifest},
			"TestInvalidPortWithoutTargetport":    {svcInvalidPortWithoutTargetportManifest, invalidPortWithoutTargetportManifest},
			"TestInvalidPortAndInvalidTargetport": {svcInvalidPortAndInvalidTargetportManifest, invalidPortAndInvalidTargetportManifest},
		},
	}
}

func (s *portRoutingTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupK8sManifest)
	s.NoError(err, "can apply setup k8s gateway manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=gw"}, time.Minute*2)
}

func (s *portRoutingTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupK8sManifest)
	s.NoError(err, "can delete setup k8s gateway manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
}

func (s *portRoutingTestingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.NoError(err, "can apply "+manifest)
	}
}

func (s *portRoutingTestingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, manifest)
		s.NoError(err, "can delete "+manifest)
	}
}

func (s *portRoutingTestingSuite) TestInvalidPortAndValidTargetport() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedHealthyResponse)
}

func (s *portRoutingTestingSuite) TestMatchPortAndTargetport() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedHealthyResponse)
}

func (s *portRoutingTestingSuite) TestMatchPodPortWithoutTargetport() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedHealthyResponse)
}

func (s *portRoutingTestingSuite) TestInvalidPortWithoutTargetport() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedServiceUnavailableResponse)
}

func (s *portRoutingTestingSuite) TestInvalidPortAndInvalidTargetport() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedServiceUnavailableResponse)
}
