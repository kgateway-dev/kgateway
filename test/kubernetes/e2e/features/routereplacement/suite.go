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
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			testdefaults.HttpbinManifest,
			setupManifest,
		},
	}

	testModes = map[string]settings.RouteReplacementMode{
		"TestStrictModeInvalidPolicyReplacement":   settings.RouteReplacementStrict,
		"TestStandardModeInvalidPolicyReplacement": settings.RouteReplacementStandard,
		"TestStrictModeInvalidMatcherDropsRoute":   settings.RouteReplacementStrict,
		"TestStrictModeInvalidRouteReplacement":    settings.RouteReplacementStrict,
	}

	testCases = map[string]*base.TestCase{
		"TestStrictModeInvalidPolicyReplacement": {
			Manifests: []string{strictModeInvalidPolicyManifest},
		},
		"TestStandardModeInvalidPolicyReplacement": {
			Manifests: []string{standardModeInvalidPolicyManifest},
		},
		"TestStrictModeInvalidMatcherDropsRoute": {
			Manifests: []string{strictModeInvalidMatcherManifest},
		},
		"TestStrictModeInvalidRouteReplacement": {
			Manifests: []string{strictModeInvalidRouteManifest},
		},
	}
)

// testingSuite is a suite of route replacement tests that verify the guardrail behavior
// for invalid route configurations in both STANDARD and STRICT modes
type testingSuite struct {
	*base.BaseTestingSuite

	// testModes maps test name to its route replacement mode
	testModes map[string]settings.RouteReplacementMode

	// original deployment state for cleanup
	originalDeployment *appsv1.Deployment
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
		testModes:        testModes,
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	// Store original deployment state for cleanup
	controllerNamespace := s.TestInstallation.Metadata.InstallNamespace

	s.originalDeployment = &appsv1.Deployment{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: controllerNamespace,
		Name:      helpers.DefaultKgatewayDeploymentName,
	}, s.originalDeployment)
	s.Require().NoError(err, "can get original controller deployment")
}

func (s *testingSuite) TearDownSuite() {
	// Restore original deployment state
	s.restoreOriginalDeployment()

	s.BaseTestingSuite.TearDownSuite()
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	mode, exists := s.testModes[testName]
	if !exists {
		s.FailNow(fmt.Sprintf("no mode configuration found for test %s", testName))
	}

	// Patch deployment with required route replacement mode
	s.patchDeploymentWithMode(mode)

	s.BaseTestingSuite.BeforeTest(suiteName, testName)
}

// TestStrictModeInvalidPolicyReplacement tests that in STRICT mode,
// routes with valid configuration but invalid custom policies are replaced with direct responses
func (s *testingSuite) TestStrictModeInvalidPolicyReplacement() {
	// Verify route status shows Accepted=False with RouteRuleDropped reason (for replacement)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		invalidPolicyRoute.Name,
		invalidPolicyRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionFalse,
	)

	// Verify that a route with an invalid policy is replaced with a 500 direct response
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
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
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		invalidPolicyRoute.Name,
		invalidPolicyRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	// Verify that the route works normally (STANDARD mode doesn't replace with 500)
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
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

// TestStrictModeInvalidMatcherDropsRoute tests that in STRICT mode,
// routes with invalid matchers are dropped entirely
func (s *testingSuite) TestStrictModeInvalidMatcherDropsRoute() {
	// Verify route status shows Accepted=False with RouteRuleDropped reason
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		invalidMatcherRoute.Name,
		invalidMatcherRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionFalse,
	)

	// Verify that the route was dropped (no route should exist, so we should get 404)
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("invalid-matcher.example.com"),
			curl.WithPort(gatewayPort),
			curl.WithPath("/headers"),
			curl.WithHeader("x-test-header", "some-value"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusNotFound,
		},
	)
}

// TestStrictModeInvalidRouteReplacement tests that in STRICT mode,
// routes with invalid built-in policies are replaced with direct responses
func (s *testingSuite) TestStrictModeInvalidRouteReplacement() {
	// Verify route status shows Accepted=False with RouteRuleDropped reason (for replacement)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		invalidConfigRoute.Name,
		invalidConfigRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionFalse,
	)

	// Verify that a route with an invalid built-in policy is replaced with a 500 direct response
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("invalid-config.example.com"),
			curl.WithPort(gatewayPort),
			curl.WithPath("/headers"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       gomega.ContainSubstring(`invalid route configuration detected and replaced with a direct response.`),
		},
	)
}

func (s *testingSuite) patchDeploymentWithMode(mode settings.RouteReplacementMode) {
	controllerNamespace := s.TestInstallation.Metadata.InstallNamespace

	currentDeployment := &appsv1.Deployment{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
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
	err = s.TestInstallation.ClusterContext.Client.Patch(s.Ctx, modifiedDeployment, client.MergeFrom(currentDeployment))
	s.Require().NoError(err, "can patch controller deployment")

	s.TestInstallation.Assertions.EventuallyPodContainerContainsEnvVar(
		s.Ctx,
		controllerNamespace,
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=kgateway",
		},
		helpers.KgatewayContainerName,
		envVar,
	)
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, controllerNamespace, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kgateway",
	})

	// Wait until there is only one pod. This way the new pod with the updated env var becomes the leader
	// and can write the correct status.
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, metav1.ObjectMeta{
		Name:      "kgateway",
		Namespace: s.TestInstallation.Metadata.InstallNamespace,
	}, gomega.Equal(1))
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		out, err := s.TestInstallation.Actions.Kubectl().GetContainerLogs(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, testdefaults.KGatewayDeployment)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to get pod logs")
		g.Expect(out).To(gomega.ContainSubstring("successfully acquired lease"))
	}, "60s", "10s").Should(gomega.Succeed())
}

func (s *testingSuite) restoreOriginalDeployment() {
	controllerNamespace := s.TestInstallation.Metadata.InstallNamespace

	// Get current deployment state
	currentDeployment := &appsv1.Deployment{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: controllerNamespace,
		Name:      helpers.DefaultKgatewayDeploymentName,
	}, currentDeployment)
	s.Require().NoError(err, "can get current controller deployment")

	// Restore original deployment
	s.originalDeployment.ResourceVersion = ""
	err = s.TestInstallation.ClusterContext.Client.Patch(s.Ctx, s.originalDeployment, client.MergeFrom(currentDeployment))
	s.Require().NoError(err, "can restore original controller deployment")

	// Wait for pods to be running again
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, controllerNamespace, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kgateway",
	})
}
