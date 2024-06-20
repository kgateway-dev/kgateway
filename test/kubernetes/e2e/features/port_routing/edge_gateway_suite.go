package port_routing

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

// glooGatewayPortRoutingTestingSuite is the entire Suite of tests for the "PortRouting" cases
type glooGatewayPortRoutingTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]testManifest
}

/*
The port routing suite sets up in the following order

SetupSuite:
 1. Create the setup apps (curl, nginx, etc.)
 2. Create Virtual Service

Each port routing test:
 1. Create Service with different port/targetport definition per test, and create corresponding Upstream resource
 2. Remove Upstream and Service, gloo proxy still exists with VirtualService, but no Upstream or Service

TearDownSuite:
 1. Deletes the setup apps (curl, nginx, etc.)
 2. Delete Virtual Service
*/
func NewEdgeGatewayApiTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &glooGatewayPortRoutingTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		manifests: map[string][]testManifest{
			"TestInvalidPortAndValidTargetport": {
				{manifestFile: upstreamInvalidPortAndValidTargetportManifest, extraArgs: []string{"-n", testInst.Metadata.InstallNamespace}},
				{manifestFile: svcInvalidPortAndValidTargetportManifest},
			},
			"TestMatchPortAndTargetport": {
				{manifestFile: upstreamMatchPortandTargetportManifest, extraArgs: []string{"-n", testInst.Metadata.InstallNamespace}},
				{manifestFile: svcMatchPortandTargetportManifest},
			},
			"TestMatchPodPortWithoutTargetport": {
				{manifestFile: upstreamMatchPodPortWithoutTargetportManifest, extraArgs: []string{"-n", testInst.Metadata.InstallNamespace}},
				{manifestFile: svcMatchPodPortWithoutTargetportManifest},
			},
			"TestInvalidPortWithoutTargetport": {
				{manifestFile: upstreamInvalidPortWithoutTargetportManifest, extraArgs: []string{"-n", testInst.Metadata.InstallNamespace}},
				{manifestFile: svcInvalidPortWithoutTargetportManifest},
			},
			"TestInvalidPortAndInvalidTargetportManifest": {
				{manifestFile: upstreamInvalidPortAndInvalidTargetportManifest, extraArgs: []string{"-n", testInst.Metadata.InstallNamespace}},
				{manifestFile: svcInvalidPortAndInvalidTargetportManifest},
			},
		},
	}
}

func (s *glooGatewayPortRoutingTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupEdgeManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err, "can apply edge routing manifest")
}

func (s *glooGatewayPortRoutingTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")

	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupEdgeManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err, "can delete edge routing manifest")
}

func (s *glooGatewayPortRoutingTestingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		// apply gloo gateway resources to gloo installation namespace
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest.manifestFile, manifest.extraArgs...)
		s.NoError(err, "can apply "+manifest.manifestFile)
	}
}

func (s *glooGatewayPortRoutingTestingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, manifest.manifestFile, manifest.extraArgs...)
		s.NoError(err, "can delete "+manifest.manifestFile)
	}
}

func (s *glooGatewayPortRoutingTestingSuite) TestInvalidPortAndValidTargetport() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("example.com"),
			curl.WithPort(80),
		},
		expectedHealthyResponse)
}

func (s *glooGatewayPortRoutingTestingSuite) TestMatchPortAndTargetport() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("example.com"),
			curl.WithPort(80),
		},
		expectedHealthyResponse)
}

func (s *glooGatewayPortRoutingTestingSuite) TestMatchPodPortWithoutTargetport() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("example.com"),
			curl.WithPort(80),
		},
		expectedHealthyResponse)
}

func (s *glooGatewayPortRoutingTestingSuite) TestInvalidPortWithoutTargetport() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("example.com"),
			curl.WithPort(80),
		},
		expectedServiceUnavailableResponse)
}

func (s *glooGatewayPortRoutingTestingSuite) TestInvalidPortAndInvalidTargetportManifest() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("example.com"),
			curl.WithPort(80),
		},
		expectedServiceUnavailableResponse)
}
