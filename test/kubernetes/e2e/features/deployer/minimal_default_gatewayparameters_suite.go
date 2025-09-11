//go:build ignore

package deployer

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewMinimalDefaultGatewayParametersTestingSuite

// minimalDefaultGatewayParametersDeployerSuite tests the "deployer" feature in situations where users have applied `null` values
// to as many of the helm values controlling the default GatewayParameters for the kgateway GatewayClass as possible.
// The "deployer" code can be found here: /internal/kgateway/deployer
type minimalDefaultGatewayParametersDeployerSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// manifests maps test name to a list of manifests to apply before the test
	manifests map[string][]string

	// manifestObjects maps a manifest file to a list of objects that are contained in that file
	manifestObjects map[string][]client.Object
}

func NewMinimalDefaultGatewayParametersTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &minimalDefaultGatewayParametersDeployerSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *minimalDefaultGatewayParametersDeployerSuite) SetupSuite() {
	s.manifests = map[string][]string{
		"TestConfigureProxiesFromGatewayParameters": {testdefaults.NginxPodManifest, gatewayWithParameters},
	}
	s.manifestObjects = map[string][]client.Object{
		testdefaults.NginxPodManifest: {testdefaults.NginxPod, testdefaults.NginxSvc},
		gatewayWithParameters:         {proxyService, proxyServiceAccount, proxyDeployment, gwParamsDefault},
	}
}

func (s *minimalDefaultGatewayParametersDeployerSuite) TearDownSuite() {
	// nothing at the moment
}

func (s *minimalDefaultGatewayParametersDeployerSuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
		s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *minimalDefaultGatewayParametersDeployerSuite) AfterTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *minimalDefaultGatewayParametersDeployerSuite) TestConfigureProxiesFromGatewayParameters() {
	deployment, err := s.testInstallation.ClusterContext.Clientset.AppsV1().Deployments(proxyDeployment.GetNamespace()).Get(s.ctx, proxyDeployment.GetName(), metav1.GetOptions{})
	s.Require().NoError(err, "can get deployment")
	s.Require().Len(deployment.Spec.Template.Spec.Containers, 1)
	secCtx := deployment.Spec.Template.Spec.Containers[0].SecurityContext
	s.Require().NotNil(secCtx)
	s.Require().Nil(secCtx.RunAsUser)
	s.Require().NotNil(secCtx.RunAsNonRoot)
	s.Require().False(*secCtx.RunAsNonRoot)
	s.Require().NotNil(secCtx.AllowPrivilegeEscalation)
	s.Require().True(*secCtx.AllowPrivilegeEscalation)
}
