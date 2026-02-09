//go:build e2e

package listener_policy

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the "ListenerPolicy" feature
type testingSuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

func NewTestingSuite(
	ctx context.Context,
	testInst *e2e.TestInstallation,
) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	// Check that the common setup manifest is applied
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply "+setupManifest)
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, exampleSvc, nginxPod)
	// Check that test app is running
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.WellKnownAppLabel + "=nginx",
	})

	// include gateway manifests for tests, so we recreate it for each test run
	s.manifests = map[string][]string{
		"TestHttpListenerPolicyAllFields":        {gatewayManifest, httpRouteManifest, allFieldsManifest},
		"TestHttpListenerPolicyServerHeader":     {gatewayManifest, httpRouteManifest, serverHeaderManifest},
		"TestPreserveHttp1HeaderCase":            {gatewayManifest, preserveHttp1HeaderCaseManifest},
		"TestAccessLogEmittedToStdout":           {gatewayManifest, httpRouteManifest, accessLogManifest},
		"TestHttpListenerPolicyClearStaleStatus": {gatewayManifest, httpRouteManifest, serverHeaderManifest},
		"TestEarlyRequestHeaderModifier":         {gatewayManifest, earlyHeaderMutationManifest},
		"TestProxyProtocol":                      {gatewayManifest, httpRouteManifest, proxyProtocolManifest},
		// RequestID configuration tests for the new RequestID feature
		// These tests use an echo server to verify x-request-id header behavior
		"TestListenerPolicyRequestId":     {gatewayManifest, requestIdEchoManifest, listenerPolicyRequestIdManifest},
		"TestHTTPListenerPolicyRequestId": {gatewayManifest, requestIdEchoManifest, httpListenerPolicyRequestIdManifest},
	}
}

func (s *testingSuite) TearDownSuite() {
	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	// Check that the common setup manifest is deleted
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.NoError(err, "can delete "+setupManifest)
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Assert().NoError(err, "can apply manifest "+manifest)
	}

	// we recreate the `Gateway` resource (and thus dynamically provision the proxy pod) for each test run
	// so let's assert the proxy svc and pod is ready before moving on
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.WellKnownAppLabel + "=gw",
	})
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, manifest)
		s.testInstallation.AssertionsT(s.T()).ExpectObjectDeleted(manifest, err, output)
	}
}

func (s *testingSuite) TestHttpListenerPolicyAllFields() {
	// Test that the HTTPListenerPolicy with all additional fields is applied correctly
	// The test verifies that the gateway is working and all policy fields are applied
	fmt.Println("TestHttpListenerPolicyAllFields")
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("Welcome to nginx!"),
		})

	// Check the health check path is working
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPath("/health_check"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.BeEmpty(),
		})
}

func (s *testingSuite) TestHttpListenerPolicyServerHeader() {
	// Test that the HTTPListenerPolicy with serverHeaderTransformation field is applied correctly
	// The test verifies that the server header is transformed as expected
	// With PassThrough, the server header should be the backend server's header (nginx/1.28.0)
	// instead of Envoy's default (envoy)
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("Welcome to nginx!"),
			Headers: map[string]any{
				"server": "nginx/1.28.0", // Should be the backend server header, not "envoy"
			},
		})
}

func (s *testingSuite) TestPreserveHttp1HeaderCase() {
	// The test verifies that the HTTP1 headers are preserved as expected in the request and response
	// The HTTPListenerPolicy ensures that the header is preserved in the request,
	// and the BackendConfigPolicy ensures that the header is preserved in the response.
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, echoService, echoDeployment)
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, echoDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=raw-header-echo",
	})
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithHeader("X-CaSeD-HeAdEr", "test"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("X-CaSeD-HeAdEr"),
			Headers: map[string]any{
				"ReSpOnSe-miXed-CaSe-hEaDeR": "Foo",
			},
		},
	)
}

func (s *testingSuite) TestAccessLogEmittedToStdout() {
	// First: trigger a 404 that SHOULD be logged (filter is GE 400)
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("not.example.com"), // not matched by HTTPRoute hostnames
			curl.WithPath("/does-not-exist"),
		},
		&matchers.HttpResponse{StatusCode: http.StatusNotFound},
	)

	// Fetch gateway pod logs and verify the 404 access log JSON fields are present
	pods, err := s.testInstallation.Actions.Kubectl().GetPodsInNsWithLabel(
		s.ctx, proxyDeployment.ObjectMeta.GetNamespace(),
		testdefaults.WellKnownAppLabel+"="+proxyDeployment.ObjectMeta.GetName(),
	)
	s.Require().NoError(err)
	s.Require().Len(pods, 1)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.testInstallation.Actions.Kubectl().GetContainerLogs(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), pods[0])
		s.Require().NoError(err)
		// Check a few key fields configured in http-listener-policy-access-log.yaml jsonFormat
		assert.Contains(c, logs, "\"method\":\"GET\"")
		assert.Contains(c, logs, "\"protocol\":\"HTTP/1.1\"")
		assert.Contains(c, logs, "\"response_code\":404")
		assert.Contains(c, logs, "\"path\":\"/does-not-exist\"")
	}, 30*time.Second, 200*time.Millisecond)

	// Second: trigger a 200 that SHOULD NOT be logged due to filter GE 400
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPath("/"),
		},
		&matchers.HttpResponse{StatusCode: http.StatusOK},
	)

	// Confirm 200 logs do not appear over a stability window as it isn't being immediately emitted
	g := gomega.NewWithT(s.T())
	g.Consistently(func() string {
		out, err := s.testInstallation.Actions.Kubectl().GetContainerLogs(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), pods[0])
		s.Require().NoError(err)
		return out
	}, 10*time.Second, 200*time.Millisecond).ShouldNot(gomega.ContainSubstring("\"response_code\":200"))
}

// TestHttpListenerPolicyClearStaleStatus verifies that stale status is cleared when targetRef becomes invalid
func (s *testingSuite) TestHttpListenerPolicyClearStaleStatus() {
	kgatewayControllerName := wellknown.DefaultGatewayControllerName
	otherControllerName := "other-controller.example.com/controller"

	// Add fake ancestor status from another controller
	s.addAncestorStatus("http-listener-policy-server-header", "default", "other-gw", otherControllerName)

	// Verify both kgateway and other controller statuses exist
	s.assertAncestorStatuses("gw", map[string]bool{
		kgatewayControllerName: true,
	})
	s.assertAncestorStatuses("other-gw", map[string]bool{
		otherControllerName: true,
	})

	// Apply policy with missing gateway target
	err := s.testInstallation.Actions.Kubectl().ApplyFile(
		s.ctx,
		httpListenerPolicyMissingTargetManifest,
	)
	s.Require().NoError(err)

	// Verify kgateway status cleared, other remains
	s.assertAncestorStatuses("gw", map[string]bool{
		kgatewayControllerName: false,
	})
	s.assertAncestorStatuses("other-gw", map[string]bool{
		otherControllerName: true,
	})
}

func (s *testingSuite) addAncestorStatus(policyName, policyNamespace, gwName, controllerName string) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	s.testInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		policy := &kgateway.ListenerPolicy{}
		err := s.testInstallation.ClusterContext.Client.Get(
			s.ctx,
			types.NamespacedName{Name: policyName, Namespace: policyNamespace},
			policy,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		// Add fake ancestor status
		fakeStatus := gwv1.PolicyAncestorStatus{
			AncestorRef:    gwv1.ParentReference{Name: gwv1.ObjectName(gwName)},
			ControllerName: gwv1.GatewayController(controllerName),
			Conditions: []metav1.Condition{
				{
					Type:               string(shared.PolicyConditionAccepted),
					Status:             metav1.ConditionTrue,
					Reason:             string(shared.PolicyReasonValid),
					Message:            "Accepted by fake controller",
					LastTransitionTime: metav1.Now(),
				},
			},
		}

		policy.Status.Ancestors = append(policy.Status.Ancestors, fakeStatus)
		err = s.testInstallation.ClusterContext.Client.Status().Update(s.ctx, policy)
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (s *testingSuite) assertAncestorStatuses(ancestorName string, expectedControllers map[string]bool) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	s.testInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		policy := &kgateway.ListenerPolicy{}
		err := s.testInstallation.ClusterContext.Client.Get(
			s.ctx,
			types.NamespacedName{Name: "http-listener-policy-server-header", Namespace: "default"},
			policy,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		foundControllers := make(map[string]bool)
		for _, ancestor := range policy.Status.Ancestors {
			if string(ancestor.AncestorRef.Name) == ancestorName {
				foundControllers[string(ancestor.ControllerName)] = true
			}
		}

		for controller, shouldExist := range expectedControllers {
			exists := foundControllers[controller]
			if shouldExist {
				g.Expect(exists).To(gomega.BeTrue(), "Expected controller %s to exist in status", controller)
			} else {
				g.Expect(exists).To(gomega.BeFalse(), "Expected controller %s to not exist in status", controller)
			}
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (s *testingSuite) TestEarlyRequestHeaderModifier() {
	// Route matches only when a specific header is present. The policy adds it early.
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			// No manual header provided; listener policy adds it early so route matches
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("Welcome to nginx!"),
		},
	)
}

// Test that enabling PROXY protocol causes plain HTTP (no PROXY header) to be rejected.
func (s *testingSuite) TestProxyProtocol() {
	// Attempt a normal HTTP request; expect curl to error (connection closed/empty reply).
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlError(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
		},
		56, // connection reset by peer
	)

	// test with PROXY protocol header; expect 200 OK
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
			curl.WithProxyProto(),
		},
		&matchers.HttpResponse{StatusCode: http.StatusOK})
}

// TestListenerPolicyRequestId tests the RequestID configuration feature when applied
// through a ListenerPolicy resource. This end-to-end test verifies that:
// 1. The RequestID configuration is properly applied to the gateway
// 2. Traffic flows correctly with the configuration in place
// 3. The x-request-id header is generated with valid UUID format
func (s *testingSuite) TestListenerPolicyRequestId() {
	// Wait for echo server to be ready
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, requestIdEchoService, requestIdEchoDeployment)
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, requestIdEchoDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=request-id-echo",
	})

	// Verify x-request-id is generated with valid UUID format when not provided
	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (8-4-4-4-12 lowercase hex digits)
	// The echo server returns all request headers in the response body, allowing us to verify
	// that Envoy properly generates the x-request-id header
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			// Verify x-request-id header was generated with valid UUID format
			Body: gomega.MatchRegexp(`(?i)x-request-id: [0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`),
		})
}

// TestHTTPListenerPolicyRequestId tests the RequestID configuration feature when applied
// through an HTTPListenerPolicy resource. This end-to-end test verifies that:
// 1. The RequestID configuration is properly applied to the gateway
// 2. Traffic flows correctly with the configuration in place
// 3. The x-request-id header is generated with valid UUID format
func (s *testingSuite) TestHTTPListenerPolicyRequestId() {
	// Wait for echo server to be ready
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, requestIdEchoService, requestIdEchoDeployment)
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, requestIdEchoDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=request-id-echo",
	})

	// Verify x-request-id is generated with valid UUID format when not provided
	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (8-4-4-4-12 lowercase hex digits)
	// The echo server returns all request headers in the response body, allowing us to verify
	// that Envoy properly generates the x-request-id header
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			// Verify x-request-id header was generated with valid UUID format
			Body: gomega.MatchRegexp(`(?i)x-request-id: [0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`),
		})
}
