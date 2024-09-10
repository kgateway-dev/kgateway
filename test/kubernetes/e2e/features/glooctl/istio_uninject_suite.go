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

var _ e2e.NewSuiteFunc = NewIstioUninjectTestingSuite

// istioUninjectTestingSuite is the entire Suite of tests for the "glooctl istio uninject" integration cases
// NOTE: This suite is not intended to be run as a standalone test suite. It applies the "glooctl unistio inject" command
// to an existing installation of Gloo Gateway where the istio-proxy and sds containers have already been injected
// and verifies that the necessary resources are removed.
type istioUninjectTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewIstioUninjectTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &istioUninjectTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *istioUninjectTestingSuite) TestCanUninject() {
	// Uninject istio with glooctl. This assumes that the istio-proxy and sds containers have already been injected
	// by the `glooctl istio inject` command in the `istioInjectTestingSuite`
	out, err := s.testInstallation.Actions.Glooctl().IstioUninject(s.ctx, s.testInstallation.Metadata.InstallNamespace, s.testInstallation.ClusterContext.KubeContext)
	s.Assert().NoError(err, "Failed to uninject istio")
	s.Assert().Contains(out, "Istio was successfully uninjected")

	matcher := gomega.And(
		gomega.Not(matchers.PodMatches(matchers.ExpectedPod{ContainerName: istio.SDSContainerName})),
		gomega.Not(matchers.PodMatches(matchers.ExpectedPod{ContainerName: istio.IstioProxyName})),
	)
	s.testInstallation.Assertions.EventuallyPodsMatches(s.ctx,
		s.testInstallation.Metadata.InstallNamespace,
		metav1.ListOptions{LabelSelector: fmt.Sprintf("gloo=%s", defaults.GatewayProxyName)},
		matcher,
	)
}
