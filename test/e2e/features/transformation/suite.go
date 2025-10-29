//go:build e2e

package transformation

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/helper"
	envoyadmincli "github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	// manifests
	simpleServiceManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	gatewayManifest                  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	transformForHeadersManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-headers.yaml")
	transformForBodyJsonManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-body-json.yaml")
	transformForBodyAsStringManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-body-as-string.yaml")
	gatewayAttachedTransformManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-attached-transform.yaml")
	transformForMatchPathManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-match-path.yaml")
	transformForMatchHeaderManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-match-header.yaml")
	transformForMatchQueryManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-match-query.yaml")
	transformForMatchMethodManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-match-method.yaml")

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	// test cases
	setup = base.TestCase{
		Manifests: []string{
			defaults.CurlPodManifest,
			simpleServiceManifest,
			gatewayManifest,
			transformForHeadersManifest,
			transformForBodyJsonManifest,
			transformForBodyAsStringManifest,
			gatewayAttachedTransformManifest,
			transformForMatchHeaderManifest,
			transformForMatchMethodManifest,
			transformForMatchPathManifest,
			transformForMatchQueryManifest,
		},
	}

	// everything is applied during setup; there are no additional test-specific manifests
	testCases = map[string]*base.TestCase{}
)

type transformationTestCase struct {
	name      string
	routeName string
	opts      []curl.Option
	resp      *testmatchers.HttpResponse
	req       *testmatchers.HttpRequest
}

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	*base.BaseTestingSuite
	// testcases that are common between the classic transformation (c++) and rustformation
	// once the rustformation is in feature parity with the classic transformation,
	// they should both just use this.
	commonTestCases []transformationTestCase
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
		[]transformationTestCase{
			{
				name:      "basic-gateway-attached",
				routeName: "gateway-attached-transform",
				resp: &testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Headers: map[string]interface{}{
						"response-gateway": "goodbye",
					},
					NotHeaders: []string{
						"x-foo-response",
					},
				},
				req: &testmatchers.HttpRequest{
					Headers: map[string]interface{}{
						"request-gateway": "hello",
					},
				},
			},
			{
				name:      "basic",
				routeName: "headers",
				opts: []curl.Option{
					curl.WithBody("hello"),
				},
				resp: &testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Headers: map[string]interface{}{
						"x-foo-response":        "notsuper",
						"x-foo-response-status": "200",
					},
					NotHeaders: []string{
						"response-gateway",
					},
				},
				req: &testmatchers.HttpRequest{
					Headers: map[string]interface{}{
						"x-foo-bar":  "foolen_5",
						"x-foo-bar2": "foolen_5",
					},
					NotHeaders: []string{
						// looks like the way we set up transformation targeting gateway, we are
						// also using RouteTransformation instead of FilterTransformation and it's
						// set , so it's set at the route table level and if there is a more specific
						// transformation (eg in vhost or prefix match), the gateway attached transformation
						// will not apply. Make sure it's not there.
						"request-gateway",
					},
				},
			},
			{
				name:      "conditional set by request header", // inja and the request_header function in use
				routeName: "headers",
				opts: []curl.Option{
					curl.WithBody("hello-world"),
					curl.WithHeader("x-add-bar", "super"),
				},
				resp: &testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Headers: map[string]interface{}{
						"x-foo-response":        "supersupersuper",
						"x-foo-response-status": "200",
					},
				},
				req: &testmatchers.HttpRequest{
					Headers: map[string]interface{}{
						"x-foo-bar":  "foolen_11",
						"x-foo-bar2": "foolen_11",
					},
					NotHeaders: []string{
						// looks like the way we set up transformation targeting gateway, we are
						// also using RouteTransformation instead of FilterTransformation and it's
						// set , so it's set at the route table level and if there is a more specific
						// transformation (eg in vhost or prefix match), the gateway attached transformation
						// will not apply. Make sure it's not there.
						"request-gateway",
					},
				},
			},
			{
				// When all matching criterion are met, path match takes precedence
				name:      "match-all",
				routeName: "match",
				opts: []curl.Option{
					curl.WithHeader("foo", "bar"),
					curl.WithPath("/path_match/index.html"),
					curl.WithQueryParameters(map[string]string{"test": "123"}),
				},
				resp: &testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Headers: map[string]interface{}{
						"x-foo-response":  "path matched",
						"x-path-response": "matched",
					},
					NotHeaders: []string{
						"response-gateway",
						"x-method-response",
						"x-header-response",
						"x-query-response",
					},
				},
				req: &testmatchers.HttpRequest{
					Headers: map[string]interface{}{
						"x-foo-request":  "path matched",
						"x-path-request": "matched",
					},
					NotHeaders: []string{
						"request-gateway",
						"x-method-request",
						"x-header-request",
						"x-query-request",
					},
				},
			},
			{
				// When all matching criterion are met except path, method match takes precedence
				name:      "match-method-header-and-query",
				routeName: "match",
				opts: []curl.Option{
					curl.WithHeader("foo", "bar"),
					curl.WithPath("/index.html"),
					curl.WithQueryParameters(map[string]string{"test": "123"}),
				},
				resp: &testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Headers: map[string]interface{}{
						"x-foo-response":    "method matched",
						"x-method-response": "matched",
					},
					NotHeaders: []string{
						"response-gateway",
						"x-path-response",
						"x-header-response",
						"x-query-response",
					},
				},
				req: &testmatchers.HttpRequest{
					Headers: map[string]interface{}{
						"x-foo-request":    "method matched",
						"x-method-request": "matched",
					},
					NotHeaders: []string{
						"request-gateway",
						"x-path-request",
						"x-header-request",
						"x-query-request",
					},
				},
			},
			{
				// When all matching criterion are met except path and method, header match takes precedence
				name:      "match-header-and-query",
				routeName: "match",
				opts: []curl.Option{
					curl.WithBody("hello"),
					curl.WithHeader("foo", "bar"),
					curl.WithPath("/index.html"),
					curl.WithQueryParameters(map[string]string{"test": "123"}),
				},
				resp: &testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Headers: map[string]interface{}{
						"x-foo-response":    "header matched",
						"x-header-response": "matched",
					},
					NotHeaders: []string{
						"response-gateway",
						"x-path-response",
						"x-method-response",
						"x-query-response",
					},
				},
				req: &testmatchers.HttpRequest{
					Headers: map[string]interface{}{
						"x-foo-request":    "header matched",
						"x-header-request": "matched",
					},
					NotHeaders: []string{
						"request-gateway",
						"x-path-request",
						"x-method-request",
						"x-query-request",
					},
				},
			},
			{
				name:      "match-query",
				routeName: "match",
				opts: []curl.Option{
					curl.WithBody("hello"),
					curl.WithPath("/index.html"),
					curl.WithQueryParameters(map[string]string{"test": "123"}),
				},
				resp: &testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Headers: map[string]interface{}{
						"x-foo-response":   "query matched",
						"x-query-response": "matched",
					},
					NotHeaders: []string{
						"response-gateway",
						"x-path-response",
						"x-method-response",
						"x-header-response",
					},
				},
				req: &testmatchers.HttpRequest{
					Headers: map[string]interface{}{
						"x-foo-request":   "query matched",
						"x-query-request": "matched",
					},
					NotHeaders: []string{
						"request-gateway",
						"x-path-request",
						"x-method-request",
						"x-header-request",
					},
				},
			},
			{
				// Interesting Note: because when a transformation attached to the gateway is set at route-table
				// level, when nothing match and envoy returns 404, that transformation won't ge applied neither!
				name:      "match-none",
				routeName: "match",
				opts: []curl.Option{
					curl.WithBody("hello"),
					curl.WithPath("/index.html"),
				},
				resp: &testmatchers.HttpResponse{
					StatusCode: http.StatusNotFound,
					Headers:    map[string]interface{}{
						// The Gateway attached transformation never apply when no route match
						//						"response-gateway": "goodbyte",
					},
					NotHeaders: []string{
						"response-gateway",
						"x-path-response",
						"x-method-response",
						"x-header-response",
						"x-query-response",
						"x-foo-response",
					},
				},
				req: &testmatchers.HttpRequest{
					Headers: map[string]interface{}{
						// The Gateway attached transformation never apply when no route match
						//						"request-gateway": "hello",
					},
					NotHeaders: []string{
						"request-gateway",
						"x-path-request",
						"x-method-request",
						"x-header-request",
						"x-foo-request",
						"x-query-request",
					},
				},
			},
		},
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	s.assertStatus()
}

func (s *testingSuite) TestGatewayWithTransformedRoute() {
	s.TestInstallation.Assertions.AssertEnvoyAdminApi(
		s.Ctx,
		proxyObjectMeta,
		s.dynamicModuleAssertion(false),
	)

	testCases := []transformationTestCase{
		{
			name:      "pull json info", // shows we parse the body as json
			routeName: "route-for-body-json",
			opts: []curl.Option{
				curl.WithBody(`{"mykey": {"myinnerkey": "myinnervalue"}}`),
				curl.WithHeader("X-Incoming-Stuff", "super"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]interface{}{
					"x-how-great":   "level_super",
					"from-incoming": "key_level_myinnervalue",
				},
			},
			// Note: for this test, there is a response body transformation setup which extracts just the headers field
			// When we create the Request Object from the echo response, we accounted for that
			req: &testmatchers.HttpRequest{
				Headers: map[string]interface{}{
					"X-Transformed-Incoming": "level_myinnervalue",
				},
			},
		},
		{
			// The default for Body parsing is AsString which translate to body passthrough (no buffering in envoy)
			// For this test, the response header transformation is set to try to use the `headers` field in the response
			// json body, because the body is never parse, so `headers` is undefine and envoy returns 400 response
			name:      "dont pull info if we dont parse json",
			routeName: "route-for-body",
			opts: []curl.Option{
				curl.WithBody(`{"mykey": {"myinnerkey": "myinnervalue"}}`),
				curl.WithHeader("X-Incoming-Stuff", "super"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusBadRequest, // bad transformation results in 400
				NotHeaders: []string{
					"x-how-great",
				},
			},
		},
		{
			name:      "dont pull json info if not json", // shows we parse the body as json
			routeName: "route-for-body-json",
			opts: []curl.Option{
				curl.WithBody("hello"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusBadRequest, // transformation should choke
			},
		},
	}
	testCases = append(testCases, s.commonTestCases...)
	s.runTestCases((testCases))
}

func (s *testingSuite) TestGatewayRustformationsWithTransformedRoute() {
	// make a copy of the original controller deployment
	controllerDeploymentOriginal := &appsv1.Deployment{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: s.TestInstallation.Metadata.InstallNamespace,
		Name:      helpers.DefaultKgatewayDeploymentName,
	}, controllerDeploymentOriginal)
	s.Assert().NoError(err, "has controller deployment")

	// add the environment variable RUSTFORMATIONS to the modified controller deployment
	rustFormationsEnvVar := corev1.EnvVar{
		Name:  "KGW_USE_RUST_FORMATIONS",
		Value: "true",
	}
	controllerDeployModified := controllerDeploymentOriginal.DeepCopy()
	controllerDeployModified.Spec.Template.Spec.Containers[0].Env = append(
		controllerDeployModified.Spec.Template.Spec.Containers[0].Env,
		rustFormationsEnvVar,
	)

	// patch the deployment
	controllerDeployModified.ResourceVersion = ""
	err = s.TestInstallation.ClusterContext.Client.Patch(s.Ctx, controllerDeployModified, client.MergeFrom(controllerDeploymentOriginal))
	s.Assert().NoError(err, "patching controller deployment")

	// wait for the changes to be reflected in pod
	s.TestInstallation.Assertions.EventuallyPodContainerContainsEnvVar(
		s.Ctx,
		s.TestInstallation.Metadata.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: defaults.ControllerLabelSelector,
		},
		helpers.KgatewayContainerName,
		rustFormationsEnvVar,
	)

	testutils.Cleanup(s.T(), func() {
		// revert to original version of deployment
		controllerDeploymentOriginal.ResourceVersion = ""
		err = s.TestInstallation.ClusterContext.Client.Patch(s.Ctx, controllerDeploymentOriginal, client.MergeFrom(controllerDeployModified))
		s.Require().NoError(err)

		// make sure the env var is removed
		s.TestInstallation.Assertions.EventuallyPodContainerDoesNotContainEnvVar(
			s.Ctx,
			s.TestInstallation.Metadata.InstallNamespace,
			metav1.ListOptions{
				LabelSelector: defaults.ControllerLabelSelector,
			},
			helpers.KgatewayContainerName,
			rustFormationsEnvVar.Name,
		)
	})

	// wait for pods to be running again, since controller deployment was patched
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, metav1.ListOptions{
		LabelSelector: defaults.ControllerLabelSelector,
	})
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, proxyObjectMeta.GetName()),
	})

	s.TestInstallation.Assertions.AssertEnvoyAdminApi(
		s.Ctx,
		proxyObjectMeta,
		s.dynamicModuleAssertion(true),
	)

	testCases := []transformationTestCase{}
	testCases = append(testCases, s.commonTestCases...)
	s.runTestCases((testCases))
}

func (s *testingSuite) runTestCases(testCases []transformationTestCase) {
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			resp := s.TestInstallation.Assertions.AssertEventualCurlReturnResponse(
				s.Ctx,
				defaults.CurlPodExecOpt,
				append(tc.opts,
					curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
					curl.WithHostHeader(fmt.Sprintf("example-%s.com", tc.routeName)),
					curl.WithPort(8080),
				),
				tc.resp)
			if resp.StatusCode == http.StatusOK {
				req, err := helper.CreateRequestFromEchoResponse(resp.Body)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(req).To(testmatchers.HaveHttpRequest(tc.req))
			} else {
				resp.Body.Close()
			}
		})
	}
}

func (s *testingSuite) assertStatus() {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	routesToCheck := []string{
		"example-route-for-headers",
		"example-route-for-body-json",
		"example-route-for-body-as-string",
		"example-route-for-gateway-attached-transform",
	}
	trafficPoliciesToCheck := []string{
		"example-traffic-policy-for-headers",
		"example-traffic-policy-for-body-json",
		"example-traffic-policy-for-body-as-string",
		"example-traffic-policy-for-gateway-attached-transform",
	}

	for i, routeName := range routesToCheck {
		trafficPolicyName := trafficPoliciesToCheck[i]

		// get the traffic policy
		s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			tp := &v1alpha1.TrafficPolicy{}
			tpObjKey := client.ObjectKey{
				Name:      trafficPolicyName,
				Namespace: "default",
			}
			err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, tpObjKey, tp)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route policy %s", tpObjKey)

			// get the route
			route := &gwv1.HTTPRoute{}
			routeObjKey := client.ObjectKey{
				Name:      routeName,
				Namespace: "default",
			}
			err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, routeObjKey, route)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route %s", routeObjKey)

			// this is the expected traffic policy status condition
			expectedCond := metav1.Condition{
				Type:               string(v1alpha1.PolicyConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(v1alpha1.PolicyReasonValid),
				Message:            reports.PolicyAcceptedMsg,
				ObservedGeneration: route.Generation,
			}

			actualPolicyStatus := tp.Status
			g.Expect(actualPolicyStatus.Ancestors).To(gomega.HaveLen(1), "should have one ancestor")
			ancestorStatus := actualPolicyStatus.Ancestors[0]
			cond := meta.FindStatusCondition(ancestorStatus.Conditions, expectedCond.Type)
			g.Expect(cond).NotTo(gomega.BeNil())
			g.Expect(cond.Status).To(gomega.Equal(expectedCond.Status))
			g.Expect(cond.Reason).To(gomega.Equal(expectedCond.Reason))
			g.Expect(cond.Message).To(gomega.Equal(expectedCond.Message))
			g.Expect(cond.ObservedGeneration).To(gomega.Equal(expectedCond.ObservedGeneration))
		}, currentTimeout, pollingInterval).Should(gomega.Succeed())
	}
}

func (s *testingSuite) dynamicModuleAssertion(shouldBeLoaded bool) func(ctx context.Context, adminClient *envoyadmincli.Client) {
	return func(ctx context.Context, adminClient *envoyadmincli.Client) {
		s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			listener, err := adminClient.GetSingleListenerFromDynamicListeners(ctx, "listener~8080")
			g.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get listener")

			// use a weak filter name check for cyclic imports
			// also we dont intend for this to be long term so dont worry about pulling it out to wellknown or something like that for now
			dynamicModuleLoaded := strings.Contains(listener.String(), "dynamic_modules/")
			if shouldBeLoaded {
				g.Expect(dynamicModuleLoaded).To(gomega.BeTrue(), fmt.Sprintf("dynamic module not loaded: %v", listener.String()))
			} else {
				g.Expect(dynamicModuleLoaded).To(gomega.BeFalse(), fmt.Sprintf("dynamic module should not be loaded: %v", listener.String()))
			}
		}).
			WithContext(ctx).
			WithTimeout(time.Second*20).
			WithPolling(time.Second).
			Should(gomega.Succeed(), "failed to get expected load of dynamic modules")
	}
}
