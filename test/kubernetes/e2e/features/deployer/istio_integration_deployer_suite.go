package deployer

import (
	"context"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/istio"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewIstioIntegrationTestingSuite

// istioIntegrationDeployerSuite is the entire Suite of tests for the "deployer" feature that relies on an Istio installation
// The "deployer" code can be found here: /projects/gateway2/deployer
type istioIntegrationDeployerSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewIstioIntegrationTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &istioIntegrationDeployerSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *istioIntegrationDeployerSuite) TestConfigureIstioIntegrationFromGatewayParameters() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, istioGatewayParametersManifestFile)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, gwParams)

		err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, deployerProvisionManifestFile)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, istioGatewayParametersManifestFile)
	s.Require().NoError(err, "can apply manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, gwParams)

	// Assert Istio integration is enabled and correct Istio image is set
	listOpts := metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gloo-proxy-gw",
	}
	matcher := gomega.And(
		matchers.PodMatches(matchers.ExpectedPod{ContainerName: istio.SDSContainerName}),
		matchers.PodMatches(matchers.ExpectedPod{ContainerName: istio.IstioProxyName}),
	)

	s.testInstallation.Assertions.EventuallyPodsMatches(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), listOpts, matcher, time.Minute*2)

}
