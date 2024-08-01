package admin_server

import (
	"context"
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/glooadminutils/admincli"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the "deployer" feature
// The "deployer" code can be found here: /projects/gateway2/deployer
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

// TestGetInputSnapshot verifies that we can query the /snapshots/input API and have it return data without an error
func (s *testingSuite) TestGetInputSnapshot() {
	s.testInstallation.Assertions.AssertGlooAdminApi(
		s.ctx,
		metav1.ObjectMeta{
			Name:      kubeutils.GlooDeploymentName,
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		},
		inputSnapshotAssertion(s.testInstallation),
	)
}

func inputSnapshotAssertion(testInstallation *e2e.TestInstallation) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			inputSnapshot, err := adminClient.GetInputSnapshot(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "error getting input snap")
			g.Expect(inputSnapshot).To(gomega.BeEquivalentTo(42000)) // this hould fail
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}
