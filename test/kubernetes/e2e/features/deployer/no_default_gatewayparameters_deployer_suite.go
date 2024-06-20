package deployer

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewNoDefaultGatewayParametersTestingSuite

// istioIntegrationDeployerSuite is the entire Suite of tests for the "deployer" feature that relies on an Istio installation
// The "deployer" code can be found here: /projects/gateway2/deployer
type noDefaultGatewayParametersDeployerSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewNoDefaultGatewayParametersTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &noDefaultGatewayParametersDeployerSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *noDefaultGatewayParametersDeployerSuite) TestConfigureProxiesFromGatewayParameters() {
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

	deployment, err := s.testInstallation.ClusterContext.Clientset.AppsV1().Deployments(proxyDeployment.GetNamespace()).Get(s.ctx, proxyDeployment.GetName(), metav1.GetOptions{})
	s.Require().NoError(err, "can get deployment")
	secCtx := deployment.Spec.Template.Spec.SecurityContext
	s.Require().NotNil(secCtx)
	s.Require().Nil(secCtx.RunAsUser)
	s.Require().NotNil(secCtx.RunAsNonRoot)
	s.Require().False(*secCtx.RunAsNonRoot)
	// AllowPrivilegeEscalation isn't on the object...?
	// s.Require().True(*secCtx.AllowPrivilegeEscalation)
}
