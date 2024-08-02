package admin_server

import (
	"context"
	"time"

	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/onsi/gomega/types"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/glooadminutils/admincli"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the "Admin Server" feature
// The "Admin Server" code can be found here: /projects/gloo/pkg/servers/admin
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

// TestGetInputSnapshotIncludesSettings verifies that we can query the /snapshots/input API and have it return Settings
// without an error.
func (s *testingSuite) TestGetInputSnapshotIncludesSettings() {
	s.testInstallation.Assertions.AssertGlooAdminApi(
		s.ctx,
		metav1.ObjectMeta{
			Name:      kubeutils.GlooDeploymentName,
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		},
		inputSnapshotContainsElement(s.testInstallation, v1.SettingsGVK, metav1.ObjectMeta{
			Name:      defaults.SettingsName,
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		}),
	)
}

// TestGetInputSnapshotIncludesEdgeApiResources verifies that we can query the /snapshots/input API and have it return Edge API
// resources without an error
func (s *testingSuite) TestGetInputSnapshotIncludesEdgeApiResources() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, upstreamManifest)
		s.NoError(err, "can delete manifest")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, upstreamManifest)
	s.Assert().NoError(err, "can apply gloo.solo.io Upstreams manifest")

	s.testInstallation.Assertions.AssertGlooAdminApi(
		s.ctx,
		metav1.ObjectMeta{
			Name:      kubeutils.GlooDeploymentName,
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		},
		inputSnapshotContainsElement(s.testInstallation, v1.UpstreamGVK, upstreamMeta),
	)
}

// TestGetInputSnapshotIncludesK8sGatewayApiResources verifies that we can query the /snapshots/input API and have it return K8s Gateway API
// resources without an error
func (s *testingSuite) TestGetInputSnapshotIncludesK8sGatewayApiResources() {
	if !s.testInstallation.Metadata.K8sGatewayEnabled {
		s.T().Skip("Installation of Gloo Gateway does not have K8s Gateway enabled, skipping test as there is nothing to test")
	}

	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, gatewayParametersManifest)
		s.NoError(err, "can delete manifest")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, gatewayParametersManifest)
	s.Assert().NoError(err, "can apply gateway.gloo.solo.io GatewayParameters manifest")

	s.testInstallation.Assertions.AssertGlooAdminApi(
		s.ctx,
		metav1.ObjectMeta{
			Name:      kubeutils.GlooDeploymentName,
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		},
		inputSnapshotContainsElement(s.testInstallation, v1alpha1.GatewayParametersGVK, gatewayParametersMeta),
	)
}

func inputSnapshotContainsElement(testInstallation *e2e.TestInstallation, gvk schema.GroupVersionKind, meta metav1.ObjectMeta) func(ctx context.Context, adminClient *admincli.Client) {
	return inputSnapshotMatches(testInstallation, gomega.ContainElement(
		gomega.And(
			gomega.HaveKeyWithValue("kind", gomega.Equal(gvk.Kind)),
			gomega.HaveKeyWithValue("apiVersion", gomega.Equal(gvk.GroupVersion().String())),
			gomega.HaveKeyWithValue("metadata", gomega.And(
				gomega.HaveKeyWithValue("name", meta.GetName()),
				gomega.HaveKeyWithValue("namespace", meta.GetNamespace()),
			)),
		),
	))
}

func inputSnapshotMatches(testInstallation *e2e.TestInstallation, inputSnapshotMatcher types.GomegaMatcher) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			inputSnapshot, err := adminClient.GetInputSnapshot(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "error getting input snapshot")
			g.Expect(inputSnapshot).NotTo(gomega.BeEmpty(), "objects are returned")
			g.Expect(inputSnapshot).To(inputSnapshotMatcher)
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}
