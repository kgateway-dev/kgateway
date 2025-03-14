package waypoint

import (
	"context"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	testdata   = filepath.Join(fsutils.MustGetThisDir(), "testdata")
	nsYAML     = filepath.Join(testdata, "common/test_namespace.yaml")
	commonYAML = filepath.Join(testdata, "common")

	testNamespace = "waypoint-test-ns"
	gwName        = "test-waypoint"

	readyTimeout = 2 * time.Minute
)

type testingSuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

// SetupSuite provides common objects used by all tests:
// * Create an ambient captured Namespace - `testNamespace`
// * Deploy a kgateway-waypoint using a Gateway resource - `gwName`
// * Deploy server (Services, Pods) - `svc-a`, `svc-b`
// * Deploy client (Pods) - `client-a`
func (s *testingSuite) SetupSuite() {
	// must apply the ns first
	err := s.testInstallation.ClusterContext.Cli.ApplyFilePath(s.ctx, nsYAML)
	if err != nil {
		s.FailNow("failed creating suite namespace", nsYAML, err)
	}
	err = s.testInstallation.ClusterContext.Cli.ApplyFilePath(s.ctx, commonYAML, "-n", testNamespace)
	if err != nil {
		s.FailNow("failed applying suite yaml", commonYAML, err)
	}

	// make sure gateway gets accepted
	s.testInstallation.Assertions.EventuallyGatewayCondition(
		s.ctx,
		gwName,
		testNamespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
		readyTimeout,
	)

	// wait for pods
	wantApps := []struct {
		lbl, val string
	}{
		{"app", "svc-a"},
		{"app", "svc-b"},
		{"app", "svc-b"},
		{"gateway.networking.k8s.io/gateway-name", gwName},
	}
	for _, app := range wantApps {
		listOpts := metav1.ListOptions{
			LabelSelector: app.lbl + "=" + app.val,
		}
		s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testNamespace, listOpts, readyTimeout)
	}
}

func (s *testingSuite) TearDownSuite() {
	err := s.testInstallation.ClusterContext.Cli.DeleteFilePath(s.ctx, commonYAML, "-n", testNamespace)
	if err != nil {
		s.Error(err)
	}
}

func (s *testingSuite) applyOrFail(fileName string, namespace string) {
	path := filepath.Join(testdata, fileName)
	if err := s.testInstallation.ClusterContext.Cli.ApplyFilePath(s.ctx, path, "-n", namespace); err != nil {
		s.FailNow("failed applying yaml", path, err)
		return
	}

	s.T().Cleanup(func() {
		if err := s.testInstallation.ClusterContext.Cli.DeleteFilePath(s.ctx, path, "-n", namespace); err != nil {
			s.FailNow("failed deleting yaml", path, err)
		}
	})
}

func (s *testingSuite) setNamespaceWaypointOrFail(ns string) {
	s.T().Cleanup(func() {
		err := s.testInstallation.ClusterContext.Cli.UnsetLabel(s.ctx, "ns", ns, "", waypointLabel)
		if err != nil {
			// this could break other tests
			s.FailNow("failed removing label", err)
		}
	})
	err := s.testInstallation.ClusterContext.Cli.SetLabel(s.ctx, "ns", ns, "", waypointLabel, gwName)
	if err != nil {
		s.FailNow("failed applying label", err)
		return
	}
}
