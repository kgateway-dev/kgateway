package headless_svc

import (
	"context"
	"path/filepath"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/utils"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type glooGatewaySuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// routingManifestFile is the file where the generated manifest files will be written for routing resources for the test suite
	routingManifestFile string
}

func NewGlooGatewayHeadlessSvcSuite(ctx context.Context, testInst *e2e.TestInstallation, outputDirectory string) suite.TestingSuite {
	manifestFile := filepath.Join(outputDirectory, GlooGatewayApiRoutingGeneratedFileName)
	return &glooGatewaySuite{
		ctx:                 ctx,
		testInstallation:    testInst,
		routingManifestFile: manifestFile,
	}
}

// SetupSuite generates manifest files for the test suite
func (s *glooGatewaySuite) SetupSuite() {
	resources := GetGlooGatewayEdgeResources(s.testInstallation.Metadata.InstallNamespace)
	err := utils.WriteResourcesToFile(resources, s.routingManifestFile)
	s.Require().NoError(err, "can write resources to file")
}

func (s *glooGatewaySuite) TestGlooGatewayRoutingHeadlessSvc() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, headlessSvcSetupManifest)
		s.NoError(err, "can delete setup manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, headlessService)

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.routingManifestFile)
		s.NoError(err, "can delete setup gloo gateway routing manifest")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, headlessSvcSetupManifest)
	s.Assert().NoError(err, "can apply setup manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, headlessService)

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.routingManifestFile)
	s.NoError(err, "can setup gloo gateway routing manifest")

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader(headlessSvcDomain),
			curl.WithPort(80),
		},
		expectedHealthyResponse)
}
