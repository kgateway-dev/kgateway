package headless_svc

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
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

func (s *headlessSvcSuite) TestConfigureRoutingHeadlessSvc() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, headlessSvcSetupManifest)
		s.NoError(err, "can delete setup manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, headlessService)

		if s.useK8sGatewayApi {
			err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, k8sApiRoutingManifest)
			s.NoError(err, "can delete setup k8s routing manifest")
			s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, k8sApiProxyDeployment, k8sApiproxyService)
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

		s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, k8sApiProxyDeployment, k8sApiproxyService)

		s.testInstallation.Assertions.AssertEventualCurlResponse(
			s.ctx,
			curlPodExecOpt,
			[]curl.Option{
				curl.WithHost(kubeutils.ServiceFQDN(k8sApiproxyService.ObjectMeta)),
				curl.WithHostHeader("example.com"),
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
				curl.WithHostHeader("example.com"),
			},
			expectedHealthyResponse)
	}

}
