package glooctl

import (
	"context"
	"fmt"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/istio"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ e2e.NewSuiteFunc = NewIstioInjectTestingSuite

// istioInjectTestingSuite is the entire Suite of tests for the "glooctl istio inject" integration cases
// NOTE: This suite is not intended to be run as a standalone test suite. It applies the "glooctl istio inject" command
// to an existing installation of Gloo Gateway and verifies that the necessary resources are created, but does not clean
// up the state after the `inject` command is run. To clean up the state run the `istioUninjectTestingSuite` after this.
type istioInjectTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewIstioInjectTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &istioInjectTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *istioInjectTestingSuite) TestCanInject() {
	// Inject istio with glooctl
	out, err := s.testInstallation.Actions.Glooctl().IstioInject(s.ctx, s.testInstallation.Metadata.InstallNamespace, s.testInstallation.ClusterContext.KubeContext)
	s.Assert().NoError(err, "Failed to inject istio")
	s.Assert().Contains(out, "Istio injection was successful!")

	matcher := gomega.And(
		matchers.PodMatches(matchers.ExpectedPod{ContainerName: istio.SDSContainerName}),
		matchers.PodMatches(matchers.ExpectedPod{ContainerName: istio.IstioProxyName}),
	)
	s.testInstallation.Assertions.EventuallyPodsMatches(s.ctx,
		s.testInstallation.Metadata.InstallNamespace,
		metav1.ListOptions{LabelSelector: fmt.Sprintf("gloo=%s", defaults.GatewayProxyName)},
		matcher,
	)
}
