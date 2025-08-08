package routereplacement

import (
	"context"
	"fmt"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

// testConfig maps a manifest to a route replacement mode
type testConfig struct {
	manifest string
	mode     settings.RouteReplacementMode
}

// testingSuite is a suite of route replacement tests that verify the guardrail behavior
// for invalid route configurations in both STANDARD and STRICT modes
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// setupManifests to apply once the suite is set up
	setupManifests []string

	// testManifests maps test name to its configuration including manifest and required mode
	testManifests map[string]testConfig

	// original deployment state for cleanup
	originalDeployment *appsv1.Deployment
}

var _ e2e.NewSuiteFunc = NewTestingSuite

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		setupManifests: []string{
			testdefaults.CurlPodManifest,
			testdefaults.HttpbinManifest,
			setupManifest,
		},
		testManifests: map[string]testConfig{
			"TestStrictModeInvalidPolicyReplacement": {
				manifest: strictModeInvalidPolicyManifest,
				mode:     settings.RouteReplacementStrict,
			},
			"TestStandardModeInvalidPolicyReplacement": {
				manifest: standardModeInvalidPolicyManifest,
				mode:     settings.RouteReplacementStandard,
			},
		},
	}
}

func (s *testingSuite) SetupSuite() {
	for _, manifest := range s.setupManifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(
		s.ctx,
		proxyDeployment,
		proxyService,
		proxyServiceAccount,
		gateway,
	)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.HttpbinDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.HttpbinLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjectMeta.GetName()),
	})

	// Store original deployment state for cleanup
	controllerNamespace := s.testInstallation.Metadata.InstallNamespace

	s.originalDeployment = &appsv1.Deployment{}
	err := s.testInstallation.ClusterContext.Client.Get(s.ctx, client.ObjectKey{
		Namespace: controllerNamespace,
		Name:      helpers.DefaultKgatewayDeploymentName,
	}, s.originalDeployment)
	s.Require().NoError(err, "can get original controller deployment")
}

func (s *testingSuite) TearDownSuite() {
	for _, manifest := range s.setupManifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err, "can delete "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsNotExist(
		s.ctx,
		proxyDeployment,
		proxyService,
		proxyServiceAccount,
		gateway,
	)

	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, testdefaults.HttpbinDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.HttpbinLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjectMeta.GetName()),
	})
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	config, exists := s.testManifests[testName]
	if !exists {
		s.FailNow(fmt.Sprintf("no configuration found for test %s", testName))
	}

	// Patch deployment with required route replacement mode
	s.patchDeploymentWithMode(config.mode)

	// Apply test-specific manifest
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, config.manifest)
	s.Require().NoError(err)
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	config, exists := s.testManifests[testName]
	if !exists {
		s.FailNow(fmt.Sprintf("no configuration found for test %s", testName))
	}

	// Clean up test-specific manifest
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, config.manifest)
	s.Require().NoError(err)

	// Restore original deployment state
	s.restoreOriginalDeployment()
}

// TestStrictModeInvalidPolicyReplacement tests that in STRICT mode,
// routes with valid configuration but invalid custom policies are replaced with direct responses
func (s *testingSuite) TestStrictModeInvalidPolicyReplacement() {
	// Verify route status shows Accepted=False with RouteRuleDropped reason (for replacement)
	s.testInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.ctx,
		invalidPolicyRoute.Name,
		invalidPolicyRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionFalse,
	)

	// Verify that a route with an invalid policy is replaced with a 500 direct response
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("invalid-policy.example.com"),
			curl.WithPort(gatewayPort),
			curl.WithPath("/headers"),
			curl.WithHeader("x-test-header", "some-value-with-policy"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       gomega.ContainSubstring(`invalid route configuration detected and replaced with a direct response.`),
		},
	)
}

// TestStandardModeInvalidPolicyReplacement tests that in STANDARD mode,
// routes with invalid policies are handled differently than in STRICT mode
func (s *testingSuite) TestStandardModeInvalidPolicyReplacement() {
	// Verify route status shows Accepted=True (STANDARD mode accepts despite invalid policy)
	s.testInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.ctx,
		invalidPolicyRoute.Name,
		invalidPolicyRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	// Verify that the route works normally (STANDARD mode doesn't replace with 500)
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("invalid-policy.example.com"),
			curl.WithPort(gatewayPort),
			curl.WithPath("/headers"),
			curl.WithHeader("x-test-header", "some-value-with-policy"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
	)
}

func (s *testingSuite) patchDeploymentWithMode(mode settings.RouteReplacementMode) {
	controllerNamespace := s.testInstallation.Metadata.InstallNamespace

	currentDeployment := &appsv1.Deployment{}
	err := s.testInstallation.ClusterContext.Client.Get(s.ctx, client.ObjectKey{
		Namespace: controllerNamespace,
		Name:      helpers.DefaultKgatewayDeploymentName,
	}, currentDeployment)
	s.Require().NoError(err, "can get current controller deployment")

	modifiedDeployment := currentDeployment.DeepCopy()
	containerIndex := -1
	for i, container := range modifiedDeployment.Spec.Template.Spec.Containers {
		if container.Name == helpers.KgatewayContainerName {
			containerIndex = i
			break
		}
	}
	if containerIndex == -1 {
		s.FailNow("kgateway container not found in deployment")
	}

	envVar := corev1.EnvVar{
		Name:  "KGW_ROUTE_REPLACEMENT_MODE",
		Value: string(mode),
	}

	var found bool
	for i, env := range modifiedDeployment.Spec.Template.Spec.Containers[containerIndex].Env {
		if env.Name == "KGW_ROUTE_REPLACEMENT_MODE" {
			modifiedDeployment.Spec.Template.Spec.Containers[containerIndex].Env[i] = envVar
			found = true
			break
		}
	}
	if !found {
		modifiedDeployment.Spec.Template.Spec.Containers[containerIndex].Env = append(
			modifiedDeployment.Spec.Template.Spec.Containers[containerIndex].Env,
			envVar,
		)
	}

	modifiedDeployment.ResourceVersion = ""
	err = s.testInstallation.ClusterContext.Client.Patch(s.ctx, modifiedDeployment, client.MergeFrom(currentDeployment))
	s.Require().NoError(err, "can patch controller deployment")

	s.testInstallation.Assertions.EventuallyPodContainerContainsEnvVar(
		s.ctx,
		controllerNamespace,
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=kgateway",
		},
		helpers.KgatewayContainerName,
		envVar,
	)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, controllerNamespace, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kgateway",
	})
}

func (s *testingSuite) restoreOriginalDeployment() {
	controllerNamespace := s.testInstallation.Metadata.InstallNamespace

	// Get current deployment state
	currentDeployment := &appsv1.Deployment{}
	err := s.testInstallation.ClusterContext.Client.Get(s.ctx, client.ObjectKey{
		Namespace: controllerNamespace,
		Name:      helpers.DefaultKgatewayDeploymentName,
	}, currentDeployment)
	s.Require().NoError(err, "can get current controller deployment")

	// Restore original deployment
	s.originalDeployment.ResourceVersion = ""
	err = s.testInstallation.ClusterContext.Client.Patch(s.ctx, s.originalDeployment, client.MergeFrom(currentDeployment))
	s.Require().NoError(err, "can restore original controller deployment")

	// Wait for pods to be running again
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, controllerNamespace, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kgateway",
	})
}
