//go:build e2e

package parallelcontrollers

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/helmutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/helper"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	setup = base.TestCase{
		Manifests: []string{
			defaults.HttpbinManifest,
			defaults.CurlPodManifest,
		},
	}

	// Empty test cases since we handle everything manually
	testCases = map[string]*base.TestCase{}
)

// testingSuite is the entire Suite of tests for the "parallelcontrollers" feature
// Tests the parallel controller architecture requirements from AGENTS.md
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// BeforeTest overrides the base suite's BeforeTest to prevent automatic test manifest application
// Setup manifests (httpbin, curl pod) are already applied in SetupSuite
// We need manual control because chart installations must happen before applying test manifests
func (s *testingSuite) BeforeTest(suiteName, testName string) {
	// Ensure httpbin pods are ready first
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		httpbinObjectMeta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + httpbinObjectMeta.GetName(),
		})

	// Ensure curl pod is ready (setup manifests are applied in SetupSuite)
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		defaults.CurlPod.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: defaults.CurlPodLabelSelector,
		})

	// Skip the base suite's automatic test manifest application
	// Each test method will manually install charts and apply manifests
}

// TearDownSuite overrides the base suite's TearDownSuite to prevent premature deletion
// of shared setup resources (httpbin, curl pod) when tests are re-run.
// The test framework's cleanup will handle final cleanup after all runs.
func (s *testingSuite) TearDownSuite() {
	// Don't delete setup manifests - let the test framework handle final cleanup
	// This prevents issues when tests are re-run due to failures
}

// applyGatewayManifests applies the gateway manifests for a specific phase
func (s *testingSuite) applyGatewayManifests(envoyGwName, envoyRouteName, envoyHostname, agwGwName, agwRouteName, agwHostname string) {
	// Apply Envoy Gateway manifest with transform
	envoyContent, err := os.ReadFile(envoyGatewayTemplate)
	s.Require().NoError(err)
	envoyTransformed := transformManifest(envoyGwName, envoyRouteName, envoyHostname)(string(envoyContent))

	gomega.Eventually(func() error {
		return s.TestInstallation.Actions.Kubectl().Apply(s.Ctx, []byte(envoyTransformed))
	}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can apply envoy gateway manifest")

	// Apply Agentgateway manifest with transform
	agwContent, err := os.ReadFile(agwGatewayTemplate)
	s.Require().NoError(err)
	agwTransformed := transformManifest(agwGwName, agwRouteName, agwHostname)(string(agwContent))

	gomega.Eventually(func() error {
		return s.TestInstallation.Actions.Kubectl().Apply(s.Ctx, []byte(agwTransformed))
	}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can apply agentgateway manifest")
}

// deleteGatewayManifests deletes the gateway resources and their dynamic resources for a specific phase
func (s *testingSuite) deleteGatewayManifests(envoyGwMeta, agwGwMeta metav1.ObjectMeta) {
	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	// Construct route names based on gateway names
	envoyRouteName := strings.Replace(envoyGwMeta.Name, "envoy-gw-", "envoy-route-", 1)
	agwRouteName := strings.Replace(agwGwMeta.Name, "agw-gw-", "agw-route-", 1)

	// Define all resources to delete
	envoyGw := &gwv1.Gateway{ObjectMeta: envoyGwMeta}
	envoyRoute := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{
		Name:      envoyRouteName,
		Namespace: envoyGwMeta.Namespace,
	}}
	envoyDeployment := &appsv1.Deployment{ObjectMeta: envoyGwMeta}
	envoyService := &corev1.Service{ObjectMeta: envoyGwMeta}
	envoyServiceAccount := &corev1.ServiceAccount{ObjectMeta: envoyGwMeta}

	agwGw := &gwv1.Gateway{ObjectMeta: agwGwMeta}
	agwRoute := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{
		Name:      agwRouteName,
		Namespace: agwGwMeta.Namespace,
	}}
	agwDeployment := &appsv1.Deployment{ObjectMeta: agwGwMeta}
	agwService := &corev1.Service{ObjectMeta: agwGwMeta}
	agwServiceAccount := &corev1.ServiceAccount{ObjectMeta: agwGwMeta}

	// Delete all resources (ignore errors if they don't exist)
	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, envoyRoute)
	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, envoyGw)

	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, agwRoute)
	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, agwGw)

	// Wait for all resources to be deleted
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx,
		envoyGw, envoyRoute, envoyDeployment, envoyService, envoyServiceAccount,
		agwGw, agwRoute, agwDeployment, agwService, agwServiceAccount,
	)
}

// TestEnvoyOnly tests that when only kgateway chart is installed, only Envoy Gateways are processed
func (s *testingSuite) TestEnvoyOnly() {
	// Install only the kgateway chart (Envoy controller)
	s.installKgatewayChart()
	defer s.uninstallKgatewayChart()

	// Apply manifests after chart installation
	s.applyGatewayManifests(
		"envoy-gw-envoy-only", "envoy-route-envoy-only", "envoy-envoy-only.example.com",
		"agw-gw-envoy-only", "agw-route-envoy-only", "agw-envoy-only.example.com",
	)
	defer s.deleteGatewayManifests(envoyGwEnvoyOnlyMeta, agwGwEnvoyOnlyMeta)

	// Assert that Envoy Gateway gets provisioned
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.Ctx,
		&appsv1.Deployment{ObjectMeta: envoyGwEnvoyOnlyMeta},
		&corev1.Service{ObjectMeta: envoyGwEnvoyOnlyMeta},
		&corev1.ServiceAccount{ObjectMeta: envoyGwEnvoyOnlyMeta},
	)

	// Assert that Envoy Gateway becomes ready
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, envoyGwEnvoyOnlyMeta, gomega.Equal(1))

	// Assert that Envoy Gateway status is Accepted and Programmed
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		envoyGwEnvoyOnlyMeta.Name,
		envoyGwEnvoyOnlyMeta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		envoyGwEnvoyOnlyMeta.Name,
		envoyGwEnvoyOnlyMeta.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)

	// Verify Envoy HTTPRoute status is updated
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyRouteEnvoyOnlyMeta.GetNamespace(),
			Name:      envoyRouteEnvoyOnlyMeta.GetName(),
		}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(route.Status.Parents).NotTo(gomega.BeEmpty(), "HTTPRoute should have parent status")
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 30).
		WithPolling(time.Second).
		Should(gomega.Succeed())

	// Wait for Envoy proxy pods to be running before making curl requests
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		envoyGwEnvoyOnlyMeta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + envoyGwEnvoyOnlyMeta.GetName(),
		})

	// Verify traffic works through Envoy Gateway
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(envoyGwEnvoyOnlyMeta)),
			curl.WithHostHeader("envoy-envoy-only.example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})

	// Assert that Agentgateway Gateway is NOT provisioned
	s.TestInstallation.AssertionsT(s.T()).Gomega.Consistently(func(g gomega.Gomega) {
		agwDeployment := &appsv1.Deployment{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: agwGwEnvoyOnlyMeta.Namespace,
			Name:      agwGwEnvoyOnlyMeta.Name,
		}, agwDeployment)
		g.Expect(err).To(gomega.HaveOccurred(), "Agentgateway Deployment should not exist when controller is disabled")

		agwService := &corev1.Service{}
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: agwGwEnvoyOnlyMeta.Namespace,
			Name:      agwGwEnvoyOnlyMeta.Name,
		}, agwService)
		g.Expect(err).To(gomega.HaveOccurred(), "Agentgateway Service should not exist when controller is disabled")
	}, "10s", "1s").Should(gomega.Succeed())

	// Verify Agentgateway Gateway status is NOT updated to Accepted
	s.TestInstallation.AssertionsT(s.T()).Gomega.Consistently(func(g gomega.Gomega) {
		agwGw := &gwv1.Gateway{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: agwGwEnvoyOnlyMeta.Namespace,
			Name:      agwGwEnvoyOnlyMeta.Name,
		}, agwGw)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		hasAcceptedTrue := false
		for _, cond := range agwGw.Status.Conditions {
			if cond.Type == string(gwv1.GatewayConditionAccepted) && cond.Status == metav1.ConditionTrue {
				hasAcceptedTrue = true
				break
			}
		}
		g.Expect(hasAcceptedTrue).To(gomega.BeFalse(), "Agentgateway Gateway should not be accepted when controller is disabled")
	}, "10s", "1s").Should(gomega.Succeed())

	// Verify that the Deployment uses the envoy chart (check container image or labels)
	s.verifyEnvoyDeployment(envoyGwEnvoyOnlyMeta)
}

// TestAgentgatewayOnly tests that when only agentgateway chart is installed, only Agentgateway Gateways are processed
func (s *testingSuite) TestAgentgatewayOnly() {
	// Install only the agentgateway chart (Agentgateway controller)
	s.installAgentgatewayChart()
	defer s.uninstallAgentgatewayChart()

	// Apply manifests after chart installation
	s.applyGatewayManifests(
		"envoy-gw-agw-only", "envoy-route-agw-only", "envoy-agw-only.example.com",
		"agw-gw-agw-only", "agw-route-agw-only", "agw-agw-only.example.com",
	)
	defer s.deleteGatewayManifests(envoyGwAgwOnlyMeta, agwGwAgwOnlyMeta)

	// Assert that Agentgateway Gateway gets provisioned
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.Ctx,
		&appsv1.Deployment{ObjectMeta: agwGwAgwOnlyMeta},
		&corev1.Service{ObjectMeta: agwGwAgwOnlyMeta},
		&corev1.ServiceAccount{ObjectMeta: agwGwAgwOnlyMeta},
	)

	// Assert that Agentgateway Gateway becomes ready
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, agwGwAgwOnlyMeta, gomega.Equal(1))

	// Assert that Agentgateway Gateway status is Accepted and Programmed
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		agwGwAgwOnlyMeta.Name,
		agwGwAgwOnlyMeta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		agwGwAgwOnlyMeta.Name,
		agwGwAgwOnlyMeta.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)

	// Verify Agentgateway HTTPRoute status is updated
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: agwRouteAgwOnlyMeta.GetNamespace(),
			Name:      agwRouteAgwOnlyMeta.GetName(),
		}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(route.Status.Parents).NotTo(gomega.BeEmpty(), "HTTPRoute should have parent status")
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 30).
		WithPolling(time.Second).
		Should(gomega.Succeed())

	// Wait for Agentgateway proxy pods to be running before making curl requests
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		agwGwAgwOnlyMeta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + agwGwAgwOnlyMeta.GetName(),
		})

	// Verify traffic works through Agentgateway Gateway
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(agwGwAgwOnlyMeta)),
			curl.WithHostHeader("agw-agw-only.example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})

	// Assert that Envoy Gateway is NOT provisioned
	s.TestInstallation.AssertionsT(s.T()).Gomega.Consistently(func(g gomega.Gomega) {
		envoyDeployment := &appsv1.Deployment{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyGwAgwOnlyMeta.Namespace,
			Name:      envoyGwAgwOnlyMeta.Name,
		}, envoyDeployment)
		g.Expect(err).To(gomega.HaveOccurred(), "Envoy Deployment should not exist when controller is disabled")

		envoyService := &corev1.Service{}
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyGwAgwOnlyMeta.Namespace,
			Name:      envoyGwAgwOnlyMeta.Name,
		}, envoyService)
		g.Expect(err).To(gomega.HaveOccurred(), "Envoy Service should not exist when controller is disabled")
	}, "10s", "1s").Should(gomega.Succeed())

	// Verify Envoy Gateway status is NOT updated to Accepted
	s.TestInstallation.AssertionsT(s.T()).Gomega.Consistently(func(g gomega.Gomega) {
		envoyGw := &gwv1.Gateway{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyGwAgwOnlyMeta.Namespace,
			Name:      envoyGwAgwOnlyMeta.Name,
		}, envoyGw)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		hasAcceptedTrue := false
		for _, cond := range envoyGw.Status.Conditions {
			if cond.Type == string(gwv1.GatewayConditionAccepted) && cond.Status == metav1.ConditionTrue {
				hasAcceptedTrue = true
				break
			}
		}
		g.Expect(hasAcceptedTrue).To(gomega.BeFalse(), "Envoy Gateway should not be accepted when controller is disabled")
	}, "10s", "1s").Should(gomega.Succeed())

	// Verify that the Deployment uses the agentgateway chart
	s.verifyAgentgatewayDeployment(agwGwAgwOnlyMeta)
}

// TestBothEnabled tests that when both charts are installed, both Gateway types work independently
func (s *testingSuite) TestBothEnabled() {
	// Install both charts
	s.installKgatewayChart()
	defer s.uninstallKgatewayChart()

	s.installAgentgatewayChart()
	defer s.uninstallAgentgatewayChart()

	// Apply manifests after chart installations
	s.applyGatewayManifests(
		"envoy-gw-both-enabled", "envoy-route-both-enabled", "envoy-both-enabled.example.com",
		"agw-gw-both-enabled", "agw-route-both-enabled", "agw-both-enabled.example.com",
	)
	defer s.deleteGatewayManifests(envoyGwBothEnabledMeta, agwGwBothEnabledMeta)

	// Assert that both Gateways get provisioned
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.Ctx,
		&appsv1.Deployment{ObjectMeta: envoyGwBothEnabledMeta},
		&corev1.Service{ObjectMeta: envoyGwBothEnabledMeta},
		&corev1.ServiceAccount{ObjectMeta: envoyGwBothEnabledMeta},
		&appsv1.Deployment{ObjectMeta: agwGwBothEnabledMeta},
		&corev1.Service{ObjectMeta: agwGwBothEnabledMeta},
		&corev1.ServiceAccount{ObjectMeta: agwGwBothEnabledMeta},
	)

	// Assert that both Gateways become ready
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, envoyGwBothEnabledMeta, gomega.Equal(1))
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, agwGwBothEnabledMeta, gomega.Equal(1))

	// Assert that Envoy Gateway status is Accepted and Programmed
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		envoyGwBothEnabledMeta.Name,
		envoyGwBothEnabledMeta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		envoyGwBothEnabledMeta.Name,
		envoyGwBothEnabledMeta.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)

	// Assert that Agentgateway Gateway status is Accepted and Programmed
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		agwGwBothEnabledMeta.Name,
		agwGwBothEnabledMeta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		agwGwBothEnabledMeta.Name,
		agwGwBothEnabledMeta.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)

	// Verify both HTTPRoute statuses are updated with correct parent Gateways
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		envoyRoute := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyRouteBothEnabledMeta.GetNamespace(),
			Name:      envoyRouteBothEnabledMeta.GetName(),
		}, envoyRoute)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(envoyRoute.Status.Parents).NotTo(gomega.BeEmpty(), "Envoy HTTPRoute should have parent status")

		// Verify the parent Gateway name and controllerName are correct
		foundEnvoyParent := false
		for _, parent := range envoyRoute.Status.Parents {
			if string(parent.ParentRef.Name) == envoyGwBothEnabledMeta.Name {
				g.Expect(parent.ControllerName).To(gomega.Equal(gwv1.GatewayController(wellknown.DefaultGatewayControllerName)),
					"Envoy HTTPRoute parent status should have correct controllerName")
				foundEnvoyParent = true
				break
			}
		}
		g.Expect(foundEnvoyParent).To(gomega.BeTrue(), "Envoy HTTPRoute should have parent status for envoy Gateway")

		agwRoute := &gwv1.HTTPRoute{}
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: agwRouteBothEnabledMeta.GetNamespace(),
			Name:      agwRouteBothEnabledMeta.GetName(),
		}, agwRoute)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(agwRoute.Status.Parents).NotTo(gomega.BeEmpty(), "Agentgateway HTTPRoute should have parent status")

		// Verify the parent Gateway name and controllerName are correct
		foundAgwParent := false
		for _, parent := range agwRoute.Status.Parents {
			if string(parent.ParentRef.Name) == agwGwBothEnabledMeta.Name {
				g.Expect(parent.ControllerName).To(gomega.Equal(gwv1.GatewayController(wellknown.DefaultAgwControllerName)),
					"Agentgateway HTTPRoute parent status should have correct controllerName")
				foundAgwParent = true
				break
			}
		}
		g.Expect(foundAgwParent).To(gomega.BeTrue(), "Agentgateway HTTPRoute should have parent status for agentgateway Gateway")
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 30).
		WithPolling(time.Second).
		Should(gomega.Succeed())

	// Wait for Envoy proxy pods to be running before making curl requests
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		envoyGwBothEnabledMeta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + envoyGwBothEnabledMeta.GetName(),
		})

	// Wait for Agentgateway proxy pods to be running before making curl requests
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		agwGwBothEnabledMeta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + agwGwBothEnabledMeta.GetName(),
		})

	// Verify traffic works through Envoy Gateway
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(envoyGwBothEnabledMeta)),
			curl.WithHostHeader("envoy-both-enabled.example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})

	// Verify traffic works through Agentgateway Gateway
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(agwGwBothEnabledMeta)),
			curl.WithHostHeader("agw-both-enabled.example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})

	// Verify that the Deployments use the correct charts
	s.verifyEnvoyDeployment(envoyGwBothEnabledMeta)
	s.verifyAgentgatewayDeployment(agwGwBothEnabledMeta)

	// Verify status entries are namespaced by controllerName
	s.verifyControllerNameInStatus()
}

// installKgatewayChart installs the kgateway chart (Envoy controller)
func (s *testingSuite) installKgatewayChart() {
	if testutils.ShouldSkipInstallAndTeardown() {
		return
	}

	// Install kgateway CRDs
	crdChartURI, err := helper.GetLocalChartPath(helmutils.CRDChartName, "")
	s.Require().NoError(err)
	err = s.TestInstallation.Actions.Helm().WithReceiver(os.Stdout).Upgrade(
		s.Ctx,
		helmutils.InstallOpts{
			CreateNamespace: true,
			ReleaseName:     helmutils.CRDChartName,
			Namespace:       s.TestInstallation.Metadata.InstallNamespace,
			ChartUri:        crdChartURI,
		})
	s.Require().NoError(err, "kgateway CRD chart install should succeed")

	// Install kgateway core chart
	chartUri, err := helper.GetLocalChartPath(helmutils.ChartName, "")
	s.Require().NoError(err)
	err = s.TestInstallation.Actions.Helm().WithReceiver(os.Stdout).Upgrade(
		s.Ctx,
		helmutils.InstallOpts{
			Namespace:       s.TestInstallation.Metadata.InstallNamespace,
			CreateNamespace: true,
			ValuesFiles:     []string{s.TestInstallation.Metadata.ProfileValuesManifestFile, s.TestInstallation.Metadata.ValuesManifestFile},
			ReleaseName:     helmutils.ChartName,
			ChartUri:        chartUri,
			ExtraArgs:       s.TestInstallation.Metadata.ExtraHelmArgs,
		})
	s.Require().NoError(err, "kgateway chart install should succeed")

	// Wait for the kgateway controller pod to be ready
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		s.TestInstallation.Metadata.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=kgateway",
		})

	// Wait for GatewayClass to be created and accepted
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		gc := &gwv1.GatewayClass{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Name: wellknown.DefaultGatewayClassName,
		}, gc)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "GatewayClass kgateway should exist")
		accepted := false
		for _, cond := range gc.Status.Conditions {
			if cond.Type == string(gwv1.GatewayClassConditionStatusAccepted) && cond.Status == metav1.ConditionTrue {
				accepted = true
				break
			}
		}
		g.Expect(accepted).To(gomega.BeTrue(), "GatewayClass kgateway should be accepted")
	}).
		WithContext(s.Ctx).
		WithTimeout(60*time.Second).
		WithPolling(time.Second).
		Should(gomega.Succeed(), "GatewayClass kgateway should be created and accepted")
}

// installAgentgatewayChart installs the agentgateway chart (Agentgateway controller)
func (s *testingSuite) installAgentgatewayChart() {
	if testutils.ShouldSkipInstallAndTeardown() {
		return
	}

	// Install agentgateway CRDs
	crdChartURI, err := helper.GetLocalChartPath(helmutils.AgentgatewayCRDChartName, "")
	s.Require().NoError(err)
	err = s.TestInstallation.Actions.Helm().WithReceiver(os.Stdout).Upgrade(
		s.Ctx,
		helmutils.InstallOpts{
			CreateNamespace: true,
			ReleaseName:     helmutils.AgentgatewayCRDChartName,
			Namespace:       s.TestInstallation.Metadata.InstallNamespace,
			ChartUri:        crdChartURI,
		})
	s.Require().NoError(err, "agentgateway CRD chart install should succeed")

	// Install agentgateway core chart
	chartUri, err := helper.GetLocalChartPath(helmutils.AgentgatewayChartName, "")
	s.Require().NoError(err)
	err = s.TestInstallation.Actions.Helm().WithReceiver(os.Stdout).Upgrade(
		s.Ctx,
		helmutils.InstallOpts{
			Namespace:       s.TestInstallation.Metadata.InstallNamespace,
			CreateNamespace: true,
			ValuesFiles: []string{
				s.TestInstallation.Metadata.ProfileValuesManifestFile,
				s.TestInstallation.Metadata.ValuesManifestFile,
				e2e.ManifestPath("agent-gateway-integration.yaml"),
			},
			ReleaseName: helmutils.AgentgatewayChartName,
			ChartUri:    chartUri,
			ExtraArgs:   s.TestInstallation.Metadata.ExtraHelmArgs,
		})
	s.Require().NoError(err, "agentgateway chart install should succeed")

	// Wait for the agentgateway controller pod to be ready
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		s.TestInstallation.Metadata.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=agentgateway",
		})

	// Wait for GatewayClass to be created and accepted
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		gc := &gwv1.GatewayClass{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Name: wellknown.DefaultAgwClassName,
		}, gc)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "GatewayClass agentgateway should exist")
		accepted := false
		for _, cond := range gc.Status.Conditions {
			if cond.Type == string(gwv1.GatewayClassConditionStatusAccepted) && cond.Status == metav1.ConditionTrue {
				accepted = true
				break
			}
		}
		g.Expect(accepted).To(gomega.BeTrue(), "GatewayClass agentgateway should be accepted")
	}).
		WithContext(s.Ctx).
		WithTimeout(60*time.Second).
		WithPolling(time.Second).
		Should(gomega.Succeed(), "GatewayClass agentgateway should be created and accepted")
}

// uninstallKgatewayChart uninstalls the kgateway chart
func (s *testingSuite) uninstallKgatewayChart() {
	if testutils.ShouldSkipInstallAndTeardown() || testutils.ShouldSkipCleanup(s.T()) {
		return
	}

	// Uninstall core chart
	err := s.TestInstallation.Actions.Helm().WithReceiver(os.Stdout).Uninstall(
		s.Ctx,
		helmutils.UninstallOpts{
			ReleaseName: helmutils.ChartName,
			Namespace:   s.TestInstallation.Metadata.InstallNamespace,
		},
	)
	if err != nil {
		s.T().Logf("Warning: failed to uninstall kgateway chart: %v", err)
	}

	// Uninstall CRD chart
	err = s.TestInstallation.Actions.Helm().WithReceiver(os.Stdout).Uninstall(
		s.Ctx,
		helmutils.UninstallOpts{
			ReleaseName: helmutils.CRDChartName,
			Namespace:   s.TestInstallation.Metadata.InstallNamespace,
		},
	)
	if err != nil {
		s.T().Logf("Warning: failed to uninstall kgateway CRD chart: %v", err)
	}
}

// uninstallAgentgatewayChart uninstalls the agentgateway chart
func (s *testingSuite) uninstallAgentgatewayChart() {
	if testutils.ShouldSkipInstallAndTeardown() || testutils.ShouldSkipCleanup(s.T()) {
		return
	}

	// Uninstall core chart
	err := s.TestInstallation.Actions.Helm().WithReceiver(os.Stdout).Uninstall(
		s.Ctx,
		helmutils.UninstallOpts{
			ReleaseName: helmutils.AgentgatewayChartName,
			Namespace:   s.TestInstallation.Metadata.InstallNamespace,
		},
	)
	if err != nil {
		s.T().Logf("Warning: failed to uninstall agentgateway chart: %v", err)
	}

	// Uninstall CRD chart
	err = s.TestInstallation.Actions.Helm().WithReceiver(os.Stdout).Uninstall(
		s.Ctx,
		helmutils.UninstallOpts{
			ReleaseName: helmutils.AgentgatewayCRDChartName,
			Namespace:   s.TestInstallation.Metadata.InstallNamespace,
		},
	)
	if err != nil {
		s.T().Logf("Warning: failed to uninstall agentgateway CRD chart: %v", err)
	}
}

// verifyEnvoyDeployment verifies that the Deployment uses the envoy chart
func (s *testingSuite) verifyEnvoyDeployment(objectMeta metav1.ObjectMeta) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		deployment := &appsv1.Deployment{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: objectMeta.Namespace,
			Name:      objectMeta.Name,
		}, deployment)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify that the deployment has Envoy containers
		// The envoy deployment should have a kgateway-proxy container (and optionally an sds container)
		hasEnvoyContainer := false
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name == "kgateway-proxy" {
				hasEnvoyContainer = true
				break
			}
		}
		g.Expect(hasEnvoyContainer).To(gomega.BeTrue(), "Deployment should have kgateway-proxy container")
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(gomega.Succeed())
}

// verifyAgentgatewayDeployment verifies that the Deployment uses the agentgateway chart
func (s *testingSuite) verifyAgentgatewayDeployment(objectMeta metav1.ObjectMeta) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		deployment := &appsv1.Deployment{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: objectMeta.Namespace,
			Name:      objectMeta.Name,
		}, deployment)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify that the deployment has agentgateway container
		hasAgentgatewayContainer := false
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name == "agentgateway" {
				hasAgentgatewayContainer = true
				break
			}
		}
		g.Expect(hasAgentgatewayContainer).To(gomega.BeTrue(), "Deployment should have agentgateway container")
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(gomega.Succeed())
}

// verifyControllerNameInStatus verifies that status entries are namespaced by controllerName
func (s *testingSuite) verifyControllerNameInStatus() {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		// Check Envoy Gateway status has correct controllerName
		envoyGw := &gwv1.Gateway{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyGwBothEnabledMeta.Namespace,
			Name:      envoyGwBothEnabledMeta.Name,
		}, envoyGw)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		// Find the Accepted condition and verify it's from the correct controller
		for _, cond := range envoyGw.Status.Conditions {
			if cond.Type == string(gwv1.GatewayConditionAccepted) && cond.Status == metav1.ConditionTrue {
				// The observed generation should be set, indicating the controller processed it
				g.Expect(cond.ObservedGeneration).NotTo(gomega.BeZero(), "Envoy Gateway should have observed generation set")
				break
			}
		}

		// Check Agentgateway Gateway status has correct controllerName
		agwGw := &gwv1.Gateway{}
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: agwGwBothEnabledMeta.Namespace,
			Name:      agwGwBothEnabledMeta.Name,
		}, agwGw)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		// Find the Accepted condition and verify it's from the correct controller
		for _, cond := range agwGw.Status.Conditions {
			if cond.Type == string(gwv1.GatewayConditionAccepted) && cond.Status == metav1.ConditionTrue {
				g.Expect(cond.ObservedGeneration).NotTo(gomega.BeZero(), "Agentgateway Gateway should have observed generation set")
				break
			}
		}

		// Check that HTTPRoute statuses have entries for their respective parent Gateways only
		envoyRoute := &gwv1.HTTPRoute{}
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyRouteBothEnabledMeta.GetNamespace(),
			Name:      envoyRouteBothEnabledMeta.GetName(),
		}, envoyRoute)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(envoyRoute.Status.Parents).To(gomega.HaveLen(1), "Envoy HTTPRoute should have exactly one parent status")
		g.Expect(string(envoyRoute.Status.Parents[0].ParentRef.Name)).To(gomega.Equal(envoyGwBothEnabledMeta.Name))
		g.Expect(envoyRoute.Status.Parents[0].ControllerName).To(gomega.Equal(gwv1.GatewayController(wellknown.DefaultGatewayControllerName)))

		agwRoute := &gwv1.HTTPRoute{}
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: agwRouteBothEnabledMeta.GetNamespace(),
			Name:      agwRouteBothEnabledMeta.GetName(),
		}, agwRoute)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(agwRoute.Status.Parents).To(gomega.HaveLen(1), "Agentgateway HTTPRoute should have exactly one parent status")
		g.Expect(string(agwRoute.Status.Parents[0].ParentRef.Name)).To(gomega.Equal(agwGwBothEnabledMeta.Name))
		g.Expect(agwRoute.Status.Parents[0].ControllerName).To(gomega.Equal(gwv1.GatewayController(wellknown.DefaultAgwControllerName)))
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 30).
		WithPolling(time.Second).
		Should(gomega.Succeed())
}
