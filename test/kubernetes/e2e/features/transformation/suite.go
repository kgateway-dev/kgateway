package transformation

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
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
	envoyadmincli "github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
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
		},
	}

	// everything is applied during setup; there are no additional test-specific manifests
	testCases = map[string]*base.TestCase{}
)

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
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

	testCases := []struct {
		name      string
		routeName string
		opts      []curl.Option
		resp      *testmatchers.HttpResponse
	}{
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
					"x-foo-response": "notsuper",
				},
				NotHeaders: []string{
					"response-gateway",
				},
			},
		},
		{
			name:      "conditional set by request header", // inja and the request_header function in use
			routeName: "headers",
			opts: []curl.Option{
				curl.WithBody("hello"),
				curl.WithHeader("x-add-bar", "super"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]interface{}{
					"x-foo-response": "supersupersuper",
				},
			},
		},
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
		},
		{
			name:      "dont pull info if we dont parse json", // shows we parse the body as json
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
	for _, tc := range testCases {
		s.TestInstallation.Assertions.AssertEventualCurlResponse(
			s.Ctx,
			defaults.CurlPodExecOpt,
			append(tc.opts,
				curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
				curl.WithHostHeader(fmt.Sprintf("example-%s.com", tc.routeName)),
				curl.WithPort(8080),
			),
			tc.resp)
	}
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

	s.T().Cleanup(func() {
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

	testCases := []struct {
		name      string
		routeName string
		opts      []curl.Option
		resp      *testmatchers.HttpResponse
	}{
		{
			name:      "basic",
			routeName: "headers",
			opts: []curl.Option{
				curl.WithBody("hello"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]interface{}{
					"x-foo-response": "notsuper",
				},
			},
		},
		{
			name:      "conditional set by request header", // inja and the request_header function in use
			routeName: "headers",
			opts: []curl.Option{
				curl.WithBody("hello"),
				curl.WithHeader("x-add-bar", "super"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]interface{}{
					"x-foo-response": "supersupersuper",
				},
			},
		},
	}
	for _, tc := range testCases {
		s.TestInstallation.Assertions.AssertEventualCurlResponse(
			s.Ctx,
			defaults.CurlPodExecOpt,
			append(tc.opts,
				curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
				curl.WithHostHeader(fmt.Sprintf("example-%s.com", tc.routeName)),
				curl.WithPort(8080),
			),
			tc.resp)
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
				dynamicModuleRouteConfigured := strings.Contains(listener.String(), "transformation/helper")
				g.Expect(dynamicModuleRouteConfigured).To(gomega.BeTrue(), fmt.Sprintf("dynamic module routespecific not loaded: %v", listener.String()))
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
