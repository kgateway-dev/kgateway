//go:build e2e

package dualcontroller

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/helmutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/helper"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
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

// testingSuite is the entire Suite of tests for the "dualcontroller" feature
// Tests the dual controller architecture requirements from AGENTS.md
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
// We need manual control because helm upgrades must happen before applying test manifests
func (s *testingSuite) BeforeTest(suiteName, testName string) {
	// Ensure curl pod is ready (setup manifests are applied in SetupSuite)
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx,
		defaults.CurlPod.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: defaults.CurlPodLabelSelector,
		})

	// Skip the base suite's automatic test manifest application
	// Each test method will manually apply its manifests after helm upgrade
}

// AfterTest overrides the base suite's AfterTest for manual cleanup
func (s *testingSuite) AfterTest(suiteName, testName string) {
	// Cleanup is handled via defer in each test method
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

	// Give Kubernetes a moment to process the resources
	time.Sleep(2 * time.Second)
}

// deleteGatewayManifests deletes the gateway resources and their dynamic resources for a specific phase
func (s *testingSuite) deleteGatewayManifests(envoyGwMeta, agwGwMeta metav1.ObjectMeta) {
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
	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, envoyDeployment)
	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, envoyService)
	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, envoyServiceAccount)

	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, agwRoute)
	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, agwGw)
	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, agwDeployment)
	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, agwService)
	s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, agwServiceAccount)

	// Wait for all resources to be deleted
	s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx,
		envoyGw, envoyRoute, envoyDeployment, envoyService, envoyServiceAccount,
		agwGw, agwRoute, agwDeployment, agwService, agwServiceAccount,
	)
}

// TestPhase1EnvoyOnly tests that when only Envoy controller is enabled, only Envoy Gateways are processed
func (s *testingSuite) TestPhase1EnvoyOnly() {
	// Note: Initial installation already has envoy.enabled=true, agentgateway.enabled=false
	// So we don't need to upgrade helm here - the controller is already configured correctly
	// We still call upgradeHelmWithFlags to ensure consistency and wait for controller readiness
	s.upgradeHelmWithFlags(true, false)

	// Apply manifests after helm upgrade
	s.applyGatewayManifests(
		"envoy-gw-phase1", "envoy-route-phase1", "envoy-phase1.example.com",
		"agw-gw-phase1", "agw-route-phase1", "agw-phase1.example.com",
	)
	defer s.deleteGatewayManifests(envoyGwPhase1Meta, agwGwPhase1Meta)

	// Assert that Envoy Gateway gets provisioned
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx,
		&appsv1.Deployment{ObjectMeta: envoyGwPhase1Meta},
		&corev1.Service{ObjectMeta: envoyGwPhase1Meta},
		&corev1.ServiceAccount{ObjectMeta: envoyGwPhase1Meta},
	)

	// Assert that Envoy Gateway becomes ready
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, envoyGwPhase1Meta, gomega.Equal(1))

	// Assert that Envoy Gateway status is Accepted and Programmed
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		envoyGwPhase1Meta.Name,
		envoyGwPhase1Meta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		envoyGwPhase1Meta.Name,
		envoyGwPhase1Meta.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)

	// Verify Envoy HTTPRoute status is updated
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: "default",
			Name:      "envoy-route-phase1",
		}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(route.Status.Parents).NotTo(gomega.BeEmpty(), "HTTPRoute should have parent status")
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 30).
		WithPolling(time.Second).
		Should(gomega.Succeed())

	// Wait for Envoy proxy pods to be running before making curl requests
	// EventuallyReadyReplicas ensures pods are ready, but we also verify they're Running
	// to catch any edge cases where pods report ready but aren't actually running
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		pods, err := s.TestInstallation.ClusterContext.Clientset.CoreV1().Pods(envoyGwPhase1Meta.GetNamespace()).List(s.Ctx, metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + envoyGwPhase1Meta.GetName(),
		})
		g.Expect(err).NotTo(gomega.HaveOccurred(), "should be able to list pods")
		g.Expect(pods.Items).NotTo(gomega.BeEmpty(), "should have at least one pod")

		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				s.T().Logf("Pod %s is not Running, phase: %s", pod.Name, pod.Status.Phase)
				// Log pod events for debugging
				events, err := s.TestInstallation.ClusterContext.Clientset.CoreV1().Events(pod.Namespace).List(s.Ctx, metav1.ListOptions{
					FieldSelector: fmt.Sprintf("involvedObject.name=%s", pod.Name),
				})
				if err == nil {
					for _, event := range events.Items {
						s.T().Logf("Pod %s event: %s - %s", pod.Name, event.Reason, event.Message)
					}
				}
			}
			g.Expect(pod.Status.Phase).To(gomega.Equal(corev1.PodRunning), "pod %s should be Running", pod.Name)
			g.Expect(pod.Status.ContainerStatuses).NotTo(gomega.BeEmpty(), "pod %s should have container statuses", pod.Name)
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if !containerStatus.Ready {
					s.T().Logf("Container %s in pod %s is not ready", containerStatus.Name, pod.Name)
					// Log container state for debugging
					if containerStatus.State.Waiting != nil {
						s.T().Logf("Container %s waiting: %s - %s", containerStatus.Name, containerStatus.State.Waiting.Reason, containerStatus.State.Waiting.Message)
					}
					if containerStatus.State.Terminated != nil {
						s.T().Logf("Container %s terminated: %s - %s (exit code: %d)", containerStatus.Name, containerStatus.State.Terminated.Reason, containerStatus.State.Terminated.Message, containerStatus.State.Terminated.ExitCode)
						// Fetch pod logs for crashed containers
						req := s.TestInstallation.ClusterContext.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
							Container: containerStatus.Name,
							TailLines: ptr.To(int64(50)),
						})
						logs, err := req.Stream(s.Ctx)
						if err == nil {
							defer logs.Close()
							logBytes := make([]byte, 4096)
							n, readErr := logs.Read(logBytes)
							if readErr == nil && n > 0 {
								s.T().Logf("Container %s logs:\n%s", containerStatus.Name, string(logBytes[:n]))
							} else if readErr != nil && readErr.Error() != "EOF" {
								s.T().Logf("Error reading logs for container %s: %v", containerStatus.Name, readErr)
							}
						} else {
							s.T().Logf("Failed to get logs for container %s: %v", containerStatus.Name, err)
						}
					}
				}
				g.Expect(containerStatus.Ready).To(gomega.BeTrue(), "container %s in pod %s should be ready", containerStatus.Name, pod.Name)
			}
		}
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Minute).
		WithPolling(time.Second).
		Should(gomega.Succeed(), "Envoy proxy pods should be running and ready")

	// Verify traffic works through Envoy Gateway
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(envoyGwPhase1Meta)),
			curl.WithHostHeader("envoy-phase1.example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})

	// Assert that Agentgateway Gateway is NOT provisioned
	s.TestInstallation.Assertions.Gomega.Consistently(func(g gomega.Gomega) {
		agwDeployment := &appsv1.Deployment{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: agwGwPhase1Meta.Namespace,
			Name:      agwGwPhase1Meta.Name,
		}, agwDeployment)
		g.Expect(err).To(gomega.HaveOccurred(), "Agentgateway Deployment should not exist when controller is disabled")

		agwService := &corev1.Service{}
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: agwGwPhase1Meta.Namespace,
			Name:      agwGwPhase1Meta.Name,
		}, agwService)
		g.Expect(err).To(gomega.HaveOccurred(), "Agentgateway Service should not exist when controller is disabled")
	}, "10s", "1s").Should(gomega.Succeed())

	// Verify Agentgateway Gateway status is NOT updated to Accepted
	s.TestInstallation.Assertions.Gomega.Consistently(func(g gomega.Gomega) {
		agwGw := &gwv1.Gateway{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: agwGwPhase1Meta.Namespace,
			Name:      agwGwPhase1Meta.Name,
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
	s.verifyEnvoyDeployment(envoyGwPhase1Meta)
}

// TestPhase2AgentgatewayOnly tests that when only Agentgateway controller is enabled, only Agentgateway Gateways are processed
func (s *testingSuite) TestPhase2AgentgatewayOnly() {
	// Upgrade helm to enable only Agentgateway controller
	s.upgradeHelmWithFlags(false, true)

	// Apply manifests after helm upgrade
	s.applyGatewayManifests(
		"envoy-gw-phase2", "envoy-route-phase2", "envoy-phase2.example.com",
		"agw-gw-phase2", "agw-route-phase2", "agw-phase2.example.com",
	)
	defer s.deleteGatewayManifests(envoyGwPhase2Meta, agwGwPhase2Meta)

	// Assert that Agentgateway Gateway gets provisioned
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx,
		&appsv1.Deployment{ObjectMeta: agwGwPhase2Meta},
		&corev1.Service{ObjectMeta: agwGwPhase2Meta},
		&corev1.ServiceAccount{ObjectMeta: agwGwPhase2Meta},
	)

	// Assert that Agentgateway Gateway becomes ready
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, agwGwPhase2Meta, gomega.Equal(1))

	// Assert that Agentgateway Gateway status is Accepted and Programmed
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		agwGwPhase2Meta.Name,
		agwGwPhase2Meta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		agwGwPhase2Meta.Name,
		agwGwPhase2Meta.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)

	// Verify Agentgateway HTTPRoute status is updated
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: "default",
			Name:      "agw-route-phase2",
		}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(route.Status.Parents).NotTo(gomega.BeEmpty(), "HTTPRoute should have parent status")
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 30).
		WithPolling(time.Second).
		Should(gomega.Succeed())

	// Wait for Agentgateway proxy pods to be running before making curl requests
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx,
		agwGwPhase2Meta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + agwGwPhase2Meta.GetName(),
		})

	// Verify traffic works through Agentgateway Gateway
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(agwGwPhase2Meta)),
			curl.WithHostHeader("agw-phase2.example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})

	// Assert that Envoy Gateway is NOT provisioned
	s.TestInstallation.Assertions.Gomega.Consistently(func(g gomega.Gomega) {
		envoyDeployment := &appsv1.Deployment{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyGwPhase2Meta.Namespace,
			Name:      envoyGwPhase2Meta.Name,
		}, envoyDeployment)
		g.Expect(err).To(gomega.HaveOccurred(), "Envoy Deployment should not exist when controller is disabled")

		envoyService := &corev1.Service{}
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyGwPhase2Meta.Namespace,
			Name:      envoyGwPhase2Meta.Name,
		}, envoyService)
		g.Expect(err).To(gomega.HaveOccurred(), "Envoy Service should not exist when controller is disabled")
	}, "10s", "1s").Should(gomega.Succeed())

	// Verify Envoy Gateway status is NOT updated to Accepted
	s.TestInstallation.Assertions.Gomega.Consistently(func(g gomega.Gomega) {
		envoyGw := &gwv1.Gateway{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyGwPhase2Meta.Namespace,
			Name:      envoyGwPhase2Meta.Name,
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
	s.verifyAgentgatewayDeployment(agwGwPhase2Meta)

	// Verify that Envoy Gateway from phase1 no longer has resources (controller disabled it)
	s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx,
		&appsv1.Deployment{ObjectMeta: envoyGwPhase1Meta},
	)
}

// TestPhase3BothEnabled tests that when both controllers are enabled, both Gateway types work independently
func (s *testingSuite) TestPhase3BothEnabled() {
	// Upgrade helm to enable both controllers
	s.upgradeHelmWithFlags(true, true)

	// Apply manifests after helm upgrade
	s.applyGatewayManifests(
		"envoy-gw-phase3", "envoy-route-phase3", "envoy-phase3.example.com",
		"agw-gw-phase3", "agw-route-phase3", "agw-phase3.example.com",
	)
	defer s.deleteGatewayManifests(envoyGwPhase3Meta, agwGwPhase3Meta)

	// Assert that both Gateways get provisioned
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx,
		&appsv1.Deployment{ObjectMeta: envoyGwPhase3Meta},
		&corev1.Service{ObjectMeta: envoyGwPhase3Meta},
		&corev1.ServiceAccount{ObjectMeta: envoyGwPhase3Meta},
		&appsv1.Deployment{ObjectMeta: agwGwPhase3Meta},
		&corev1.Service{ObjectMeta: agwGwPhase3Meta},
		&corev1.ServiceAccount{ObjectMeta: agwGwPhase3Meta},
	)

	// Assert that both Gateways become ready
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, envoyGwPhase3Meta, gomega.Equal(1))
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, agwGwPhase3Meta, gomega.Equal(1))

	// Assert that Envoy Gateway status is Accepted and Programmed
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		envoyGwPhase3Meta.Name,
		envoyGwPhase3Meta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		envoyGwPhase3Meta.Name,
		envoyGwPhase3Meta.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)

	// Assert that Agentgateway Gateway status is Accepted and Programmed
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		agwGwPhase3Meta.Name,
		agwGwPhase3Meta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		agwGwPhase3Meta.Name,
		agwGwPhase3Meta.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)

	// Verify both HTTPRoute statuses are updated
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		envoyRoute := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: "default",
			Name:      "envoy-route-phase3",
		}, envoyRoute)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(envoyRoute.Status.Parents).NotTo(gomega.BeEmpty(), "Envoy HTTPRoute should have parent status")

		agwRoute := &gwv1.HTTPRoute{}
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: "default",
			Name:      "agw-route-phase3",
		}, agwRoute)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(agwRoute.Status.Parents).NotTo(gomega.BeEmpty(), "Agentgateway HTTPRoute should have parent status")
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 30).
		WithPolling(time.Second).
		Should(gomega.Succeed())

	// Wait for Envoy proxy pods to be running before making curl requests
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx,
		envoyGwPhase3Meta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + envoyGwPhase3Meta.GetName(),
		})

	// Wait for Agentgateway proxy pods to be running before making curl requests
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx,
		agwGwPhase3Meta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + agwGwPhase3Meta.GetName(),
		})

	// Verify traffic works through Envoy Gateway
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(envoyGwPhase3Meta)),
			curl.WithHostHeader("envoy-phase3.example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})

	// Verify traffic works through Agentgateway Gateway
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(agwGwPhase3Meta)),
			curl.WithHostHeader("agw-phase3.example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})

	// Verify that the Deployments use the correct charts
	s.verifyEnvoyDeployment(envoyGwPhase3Meta)
	s.verifyAgentgatewayDeployment(agwGwPhase3Meta)

	// Verify status entries are namespaced by controllerName
	s.verifyControllerNameInStatus()
}

// upgradeHelmWithFlags performs a helm upgrade with the specified enable flags
func (s *testingSuite) upgradeHelmWithFlags(enableEnvoy, enableAgentgateway bool) {
	chartUri, err := helper.GetLocalChartPath(helmutils.ChartName, "")
	s.Require().NoError(err)

	extraArgs := []string{
		"--set", fmt.Sprintf("envoy.enabled=%t", enableEnvoy),
		"--set", fmt.Sprintf("agentgateway.enabled=%t", enableAgentgateway),
	}

	// Merge with existing extra args from test installation, but filter out conflicting flags
	// ExtraHelmArgs comes in pairs: "--set" followed by "key=value"
	for i := 0; i < len(s.TestInstallation.Metadata.ExtraHelmArgs); i++ {
		arg := s.TestInstallation.Metadata.ExtraHelmArgs[i]
		if arg == "--set" && i+1 < len(s.TestInstallation.Metadata.ExtraHelmArgs) {
			value := s.TestInstallation.Metadata.ExtraHelmArgs[i+1]
			// Skip envoy.enabled and agentgateway.enabled flags since we're setting them explicitly
			if strings.HasPrefix(value, "envoy.enabled=") || strings.HasPrefix(value, "agentgateway.enabled=") {
				i++ // Skip both "--set" and the value
				continue
			}
		}
		extraArgs = append(extraArgs, arg)
	}

	err = s.TestInstallation.Actions.Helm().WithReceiver(os.Stdout).Upgrade(
		s.Ctx,
		helmutils.InstallOpts{
			Namespace:       s.TestInstallation.Metadata.InstallNamespace,
			CreateNamespace: true,
			ValuesFiles:     []string{s.TestInstallation.Metadata.ProfileValuesManifestFile, s.TestInstallation.Metadata.ValuesManifestFile},
			ReleaseName:     helmutils.ChartName,
			ChartUri:        chartUri,
			ExtraArgs:       extraArgs,
		})
	s.Require().NoError(err, "helm upgrade should succeed")

	// Wait for the kgateway controller pod to be ready after helm upgrade
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx,
		s.TestInstallation.Metadata.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=kgateway",
		})

	// Wait for GatewayClasses to be created/updated based on enable flags
	// This ensures the controller has fully reconciled before we apply Gateways
	if enableEnvoy {
		s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			gc := &gwv1.GatewayClass{}
			err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
				Name: "kgateway",
			}, gc)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "GatewayClass kgateway should exist")
			// Verify the GatewayClass is accepted (controller is ready to process Gateways)
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

	if enableAgentgateway {
		s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			gc := &gwv1.GatewayClass{}
			err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
				Name: "agentgateway",
			}, gc)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "GatewayClass agentgateway should exist")
			// Verify the GatewayClass is accepted (controller is ready to process Gateways)
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

	// Give the controller additional time to fully reconcile after GatewayClass creation
	// This ensures xDS is ready and controller can properly provision proxy pods
	time.Sleep(5 * time.Second)
}

// verifyEnvoyDeployment verifies that the Deployment uses the envoy chart
func (s *testingSuite) verifyEnvoyDeployment(objectMeta metav1.ObjectMeta) {
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
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
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		deployment := &appsv1.Deployment{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: objectMeta.Namespace,
			Name:      objectMeta.Name,
		}, deployment)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify that the deployment has agentgateway container
		hasAgentgatewayContainer := false
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name == "agent-gateway" {
				hasAgentgatewayContainer = true
				break
			}
		}
		g.Expect(hasAgentgatewayContainer).To(gomega.BeTrue(), "Deployment should have agent-gateway container")
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(gomega.Succeed())
}

// verifyControllerNameInStatus verifies that status entries are namespaced by controllerName
func (s *testingSuite) verifyControllerNameInStatus() {
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		// Check Envoy Gateway status has correct controllerName
		envoyGw := &gwv1.Gateway{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: envoyGwPhase3Meta.Namespace,
			Name:      envoyGwPhase3Meta.Name,
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
			Namespace: agwGwPhase3Meta.Namespace,
			Name:      agwGwPhase3Meta.Name,
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
			Namespace: "default",
			Name:      "envoy-route-phase3",
		}, envoyRoute)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(envoyRoute.Status.Parents).To(gomega.HaveLen(1), "Envoy HTTPRoute should have exactly one parent status")
		g.Expect(string(envoyRoute.Status.Parents[0].ParentRef.Name)).To(gomega.Equal(envoyGwPhase3Meta.Name))
		g.Expect(envoyRoute.Status.Parents[0].ControllerName).To(gomega.Equal(gwv1.GatewayController(wellknown.DefaultGatewayControllerName)))

		agwRoute := &gwv1.HTTPRoute{}
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: "default",
			Name:      "agw-route-phase3",
		}, agwRoute)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(agwRoute.Status.Parents).To(gomega.HaveLen(1), "Agentgateway HTTPRoute should have exactly one parent status")
		g.Expect(string(agwRoute.Status.Parents[0].ParentRef.Name)).To(gomega.Equal(agwGwPhase3Meta.Name))
		g.Expect(agwRoute.Status.Parents[0].ControllerName).To(gomega.Equal(gwv1.GatewayController(wellknown.DefaultAgwControllerName)))
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Second * 30).
		WithPolling(time.Second).
		Should(gomega.Succeed())
}
