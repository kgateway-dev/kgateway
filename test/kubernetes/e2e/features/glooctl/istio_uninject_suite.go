package glooctl

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// istioUninjectTestingSuite is the entire Suite of tests for the "glooctl istio uninject" integration cases
type istioUninjectTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	glooctlPath string
}

func NewIstioUninjectTestingSuite(ctx context.Context, testInst *e2e.TestInstallation, glooctlPath string) suite.TestingSuite {
	return &istioUninjectTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		glooctlPath:      glooctlPath,
	}
}

func (s *istioUninjectTestingSuite) TestCanUninject() {
	// Uninject istio with glooctl
	uninjectCmd := exec.Command(s.glooctlPath, "istio", "uninject",
		"--namespace", s.testInstallation.Metadata.InstallNamespace,
		"--kube-context", s.testInstallation.TestCluster.ClusterContext.KubeContext)
	out, err := uninjectCmd.CombinedOutput()
	s.Assert().NoError(err, "Failed to uninject istio")
	s.Assert().Contains(string(out), "Istio was successfully uninjected")

	matcher := gomega.And(gomega.Not(assertions.PodHasContainersMatcher("sds")), gomega.Not(assertions.PodHasContainersMatcher("istio-proxy")))
	s.testInstallation.Assertions.EventuallyPodsMatches(s.ctx,
		s.testInstallation.Metadata.InstallNamespace,
		metav1.ListOptions{LabelSelector: fmt.Sprintf("gloo=%s", defaults.GatewayProxyName)},
		matcher,
	)
}
