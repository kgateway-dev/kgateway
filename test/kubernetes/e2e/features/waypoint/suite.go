package waypoint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
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
	ctx                context.Context
	testInstallation   *e2e.TestInstallation
	ingressUseWaypoint bool
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:                ctx,
		testInstallation:   testInst,
		ingressUseWaypoint: false,
	}
}

func NewIngressTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:                ctx,
		testInstallation:   testInst,
		ingressUseWaypoint: true,
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
		{"app", "curl"},
		{"app", "notcurl"},
		{"gateway.networking.k8s.io/gateway-name", gwName},
	}
	for _, app := range wantApps {
		listOpts := metav1.ListOptions{
			LabelSelector: app.lbl + "=" + app.val,
		}
		s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testNamespace, listOpts, readyTimeout)
	}

	for _, app := range wantApps {
		pods, err := s.testInstallation.ClusterContext.Clientset.CoreV1().Pods(testNamespace).List(
			s.ctx,
			metav1.ListOptions{LabelSelector: app.lbl + "=" + app.val},
		)
		if err != nil {
			s.T().Logf("Error listing pods with label %s=%s: %v", app.lbl, app.val, err)
			continue
		}
		s.T().Logf("Found %d pods with label %s=%s", len(pods.Items), app.lbl, app.val)
		for _, pod := range pods.Items {
			s.T().Logf("Pod %s status: %s", pod.Name, pod.Status.Phase)
		}
	}

	// If it's a suite testing with KGW_INGRESS_USE_WAYPOINTS disabled (enabled by default),
	// we set the env var in the controller deployment for the tests
	if !s.ingressUseWaypoint {
		s.setDeploymentEnvVariable("KGW_INGRESS_USE_WAYPOINTS", "false")
	}
}

func (s *testingSuite) TearDownSuite() {
	err := s.testInstallation.ClusterContext.Cli.DeleteFilePath(s.ctx, commonYAML, "-n", testNamespace)
	if err != nil {
		s.Error(err)
	}
}

func (s *testingSuite) setDeploymentEnvVariable(name, value string) {
	controllerNamespace, ok := os.LookupEnv(testutils.InstallNamespace)
	if !ok {
		s.FailNow(fmt.Sprintf("%s environment variable not set", testutils.InstallNamespace))
	}

	// make a copy of the original controller deployment
	controllerDeploymentOriginal := &appsv1.Deployment{}
	err := s.testInstallation.ClusterContext.Client.Get(s.ctx, client.ObjectKey{
		Namespace: controllerNamespace,
		Name:      helpers.DefaultKgatewayDeploymentName,
	}, controllerDeploymentOriginal)
	s.Assert().NoError(err, "has controller deployment")

	// add the environment variable to the modified controller deployment
	envVarToAdd := corev1.EnvVar{
		Name:  name,
		Value: value,
	}
	controllerDeployModified := controllerDeploymentOriginal.DeepCopy()
	controllerDeployModified.Spec.Template.Spec.Containers[0].Env = append(
		controllerDeployModified.Spec.Template.Spec.Containers[0].Env,
		envVarToAdd,
	)

	// patch the deployment
	controllerDeployModified.ResourceVersion = ""
	err = s.testInstallation.ClusterContext.Client.Patch(s.ctx, controllerDeployModified, client.MergeFrom(controllerDeploymentOriginal))
	s.Assert().NoError(err, "patching controller deployment")

	// wait for the changes to be reflected in pod
	s.testInstallation.Assertions.EventuallyPodContainerContainsEnvVar(
		s.ctx,
		controllerNamespace,
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=kgateway",
		},
		helpers.KgatewayContainerName,
		envVarToAdd,
	)

	s.T().Cleanup(func() {
		// revert to original version of deployment
		controllerDeploymentOriginal.ResourceVersion = ""
		err = s.testInstallation.ClusterContext.Client.Patch(s.ctx, controllerDeploymentOriginal, client.MergeFrom(controllerDeployModified))
		s.Require().NoError(err)

		// make sure the env var is removed
		s.testInstallation.Assertions.EventuallyPodContainerDoesNotContainEnvVar(
			s.ctx,
			controllerNamespace,
			metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/name=kgateway",
			},
			helpers.KgatewayContainerName,
			envVarToAdd.Name,
		)
	})

	// wait for pods to be running again, since controller deployment was patched
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, controllerNamespace, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kgateway",
	})
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
	s.useWaypointLabelForTest("ns", ns, "")
}
