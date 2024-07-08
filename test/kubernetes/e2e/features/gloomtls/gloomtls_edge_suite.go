package gloomtls

import (
	"context"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/istio"
	"github.com/solo-io/gloo/test/gomega/matchers"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

// gloomtlsEdgeGatewayTestingSuite is the entire Suite of tests for the "PortRouting" cases
type gloomtlsEdgeGatewayTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	//manifests map[string][]testManifest
}

func NewGloomtlsEdgeGatewayApiTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &gloomtlsEdgeGatewayTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		//manifests: map[string][]testManifest{
		//	"TestInvalidPortAndValidTargetport": {
		//		{manifestFile: upstreamInvalidPortAndValidTargetportManifest, extraArgs: []string{"-n", testInst.Metadata.InstallNamespace}},
		//		{manifestFile: svcInvalidPortAndValidTargetportManifest},
		//	},
		//},
	}
}

func (s *gloomtlsEdgeGatewayTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply setup manifest")

}

func (s *gloomtlsEdgeGatewayTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
}

//func (s *gloomtlsEdgeGatewayTestingSuite) BeforeTest(suiteName, testName string) {
//	manifests, ok := s.manifests[testName]
//	if !ok {
//		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
//	}
//
//	for _, manifest := range manifests {
//		// apply gloo gateway resources to gloo installation namespace
//		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest.manifestFile, manifest.extraArgs...)
//		s.NoError(err, "can apply "+manifest.manifestFile)
//	}
//}
//
//func (s *gloomtlsEdgeGatewayTestingSuite) AfterTest(suiteName, testName string) {
//	manifests, ok := s.manifests[testName]
//	if !ok {
//		s.FailNow("no manifests found for " + testName)
//	}
//
//	for _, manifest := range manifests {
//		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, manifest.manifestFile, manifest.extraArgs...)
//		s.NoError(err, "can delete "+manifest.manifestFile)
//	}
//}

func (s *gloomtlsEdgeGatewayTestingSuite) TestRouteSecureRequestToUpstream() {
	// Check sds containter is present
	listOpts := metav1.ListOptions{
		LabelSelector: "gloo",
	}
	matcher := gomega.And(
		matchers.PodMatches(matchers.ExpectedPod{ContainerName: istio.SDSContainerName}),
	)

	s.testInstallation.Assertions.EventuallyPodsMatches(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), listOpts, matcher, time.Minute*2)

	// Check curl works
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("example.com"),
			curl.WithPort(80),
		},
		expectedHealthyResponse)
}
