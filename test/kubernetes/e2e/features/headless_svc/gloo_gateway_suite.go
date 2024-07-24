package headless_svc

import (
	"context"
	"path/filepath"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloo_defaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	testutilsresources "github.com/solo-io/gloo/test/kubernetes/testutils/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ e2e.NewSuiteFunc = NewEdgeGatewayHeadlessSvcSuite

type edgeGatewaySuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// routingManifestFile is the file where the generated manifest files will be written for routing resources for the test suite
	routingManifestFile string
}

func NewEdgeGatewayHeadlessSvcSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	manifestFile := filepath.Join(testInst.GeneratedFiles.TempDir, EdgeGatewayApiRoutingGeneratedFileName)
	return &edgeGatewaySuite{
		ctx:                 ctx,
		testInstallation:    testInst,
		routingManifestFile: manifestFile,
	}
}

// SetupSuite generates manifest files for the test suite
func (s *edgeGatewaySuite) SetupSuite() {
	gwResources := GetEdgeGatewayResources(s.testInstallation.Metadata.InstallNamespace)
	err := testutilsresources.WriteResourcesToFile(gwResources, s.routingManifestFile)
	s.Require().NoError(err, "can write resources to file")
}

func (s *edgeGatewaySuite) TestEdgeGatewayRoutingHeadlessSvc() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, headlessSvcSetupManifest)
		s.NoError(err, "can delete setup manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, headlessService)

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, s.routingManifestFile)
		s.NoError(err, "can delete setup Edge Gateway API routing manifest")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, headlessSvcSetupManifest)
	s.Assert().NoError(err, "can apply setup manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, headlessService)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, headlessService.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, s.routingManifestFile)
	s.NoError(err, "can setup Edge Gateway API routing manifest")
	// check status of the VirtualService and Upstream to ensure they are accepted
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, "headless-nginx-upstream", clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, "headless-vs", clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)

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
