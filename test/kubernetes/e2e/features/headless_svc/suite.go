package headless_svc

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/utils"
)

type headlessSvcSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	useK8sGatewayApi bool
}

func NewHeadlessSvcTestingSuite(ctx context.Context, testInst *e2e.TestInstallation, useK8sGatewayApi bool) suite.TestingSuite {
	return &headlessSvcSuite{
		ctx:              ctx,
		testInstallation: testInst,
		useK8sGatewayApi: useK8sGatewayApi,
	}
}

// SetupSuite generates manifest files for the test suite
func (s *headlessSvcSuite) SetupSuite() {
	if s.useK8sGatewayApi {
		// use the k8s gateway api resources
		resources := []client.Object{gw, headlessSvcHTTPRoute}
		err := utils.WriteResourcesToFile(resources, k8sApiRoutingManifest)
		s.Require().NoError(err, "can write resources to file")
	} else {
		resources := getClassicEdgeResources(s.testInstallation.Metadata.InstallNamespace)
		err := utils.WriteResourcesToFile(resources, classicApiRoutingManifest)
		s.Require().NoError(err, "can write resources to file")
	}
}

func (s *headlessSvcSuite) TestConfigureRoutingHeadlessSvc() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, headlessSvcSetupManifest)
		s.NoError(err, "can delete setup manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, headlessService)

		if s.useK8sGatewayApi {
			err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, k8sApiRoutingManifest)
			s.NoError(err, "can delete setup k8s routing manifest")
			s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, k8sApiProxyDeployment, k8sApiProxyService)
		} else {
			err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, classicApiRoutingManifest)
			s.NoError(err, "can delete setup classic routing manifest")
		}
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, headlessSvcSetupManifest)
	s.Assert().NoError(err, "can apply setup manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, headlessService)

	if s.useK8sGatewayApi {
		err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, k8sApiRoutingManifest)
		s.NoError(err, "can setup k8s routing manifest")

		s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, k8sApiProxyDeployment, k8sApiProxyService)

		s.testInstallation.Assertions.AssertEventualCurlResponse(
			s.ctx,
			curlPodExecOpt,
			[]curl.Option{
				curl.WithHost(kubeutils.ServiceFQDN(k8sApiProxyService.ObjectMeta)),
				// The host header must match the hostnames in the HTTPRoute
				curl.WithHostHeader(headlessSvcDomain),
				curl.WithPort(80),
			},
			expectedHealthyResponse)
	} else {
		err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, classicApiRoutingManifest)
		s.NoError(err, "can setup classic routing manifest")

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
}
