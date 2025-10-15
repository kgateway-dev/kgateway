//go:build ignore && e2e

package deployer

import (
	"context"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/internal/gloo/cli/pkg/cmd/istio"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewIstioIntegrationTestingSuite

// istioIntegrationDeployerSuite is the entire Suite of tests for the "deployer" feature that relies on an Istio installation
// The "deployer" code can be found here: /internal/kgateway/deployer
type istioIntegrationDeployerSuite struct {
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

func NewIstioIntegrationTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &istioIntegrationDeployerSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *istioIntegrationDeployerSuite) SetupSuite() {
	s.manifests = map[string][]string{
		"TestConfigureIstioIntegrationFromGatewayParameters": {testdefaults.NginxPodManifest, istioGatewayParameters},
	}
	s.manifestObjects = map[string][]client.Object{
		testdefaults.NginxPodManifest: {testdefaults.NginxPod, testdefaults.NginxSvc},
		istioGatewayParameters:        {proxyService, proxyServiceAccount, proxyDeployment, gwParamsDefault},
	}
}

func (s *istioIntegrationDeployerSuite) TearDownSuite() {
	// nothing at the moment
}

func (s *istioIntegrationDeployerSuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
		s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *istioIntegrationDeployerSuite) AfterTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *istioIntegrationDeployerSuite) TestConfigureIstioIntegrationFromGatewayParameters() {
	// Assert Istio integration is enabled and correct Istio image is set
	listOpts := metav1.ListOptions{
		LabelSelector: testdefaults.WellKnownAppLabel + "=gw",
	}
	matcher := gomega.And(
		matchers.PodMatches(matchers.ExpectedPod{ContainerName: istio.SDSContainerName}),
		matchers.PodMatches(matchers.ExpectedPod{ContainerName: istio.IstioProxyName}),
	)

	s.testInstallation.Assertions.EventuallyPodsMatches(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), listOpts, matcher, time.Minute*2)
}
