//go:build ignore

package admin_server

import (
	"context"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	v1 "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/defaults"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
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
		s.testInstallation.Assertions.InputSnapshotContainsElement(v1.SettingsGVK, metav1.ObjectMeta{
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
		s.testInstallation.Assertions.InputSnapshotContainsElement(v1.UpstreamGVK, upstreamMeta),
	)
}

// TestGetInputSnapshotIncludesK8sGatewayApiResources verifies that we can query the /snapshots/input API and have it return K8s Gateway API
// resources without an error
func (s *testingSuite) TestGetInputSnapshotIncludesK8sGatewayApiResources() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, gatewayParametersManifest)
		s.NoError(err, "can delete manifest")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, gatewayParametersManifest)
	s.Assert().NoError(err, "can apply gateway.kgateway.dev GatewayParameters manifest")

	s.testInstallation.Assertions.AssertGlooAdminApi(
		s.ctx,
		metav1.ObjectMeta{
			Name:      kubeutils.GlooDeploymentName,
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		},
		s.testInstallation.Assertions.InputSnapshotContainsElement(v1alpha1.GatewayParametersGVK, gatewayParametersMeta),
	)
}
