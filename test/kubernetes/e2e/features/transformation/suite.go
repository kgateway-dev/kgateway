package transformation

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	envoyadmincli "github.com/kgateway-dev/kgateway/v2/pkg/utils/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
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
		Resources: []client.Object{
			// resources from curl manifest
			defaults.CurlPod,
			// resources from service manifest
			simpleSvc, simpleDeployment,
			// resources from gateway manifest
			gateway, gwp,
			// deployer-generated resources
			proxyDeployment, proxyService, proxyServiceAccount,
			// routes and traffic policies
			routeForHeaders, routeForBodyJson, routeForBodyAsString, routeBasic,
			trafficPolicyForHeaders, trafficPolicyForBodyJson, trafficPolicyForBodyAsString, trafficPolicyForGatewayAttachedTransform,
		},
	}

	// everything is applied during setup; there are no additional test-specific manifests
	testCases = map[string]base.TestCase{}
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

	s.assertStatus(metav1.Condition{
		Type:               string(v1alpha1.PolicyConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(v1alpha1.PolicyReasonValid),
		Message:            reports.PolicyAcceptedMsg,
		ObservedGeneration: routeForHeaders.Generation,
	})
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

func (s *testingSuite) assertStatus(expected metav1.Condition) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	p := s.TestInstallation.Assertions
	p.Gomega.Eventually(func(g gomega.Gomega) {
		be := &v1alpha1.TrafficPolicy{}
		objKey := client.ObjectKeyFromObject(trafficPolicyForHeaders)
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, objKey, be)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route policy %s", objKey)
		objKey = client.ObjectKeyFromObject(trafficPolicyForBodyJson)
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, objKey, be)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route policy %s", objKey)
		objKey = client.ObjectKeyFromObject(trafficPolicyForBodyAsString)
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, objKey, be)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route policy %s", objKey)
		objKey = client.ObjectKeyFromObject(trafficPolicyForGatewayAttachedTransform)
		err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, objKey, be)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route policy %s", objKey)

		actual := be.Status
		g.Expect(actual.Ancestors).To(gomega.HaveLen(1), "should have one ancestor")
		ancestorStatus := actual.Ancestors[0]
		cond := meta.FindStatusCondition(ancestorStatus.Conditions, expected.Type)
		g.Expect(cond).NotTo(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(expected.Status))
		g.Expect(cond.Reason).To(gomega.Equal(expected.Reason))
		g.Expect(cond.Message).To(gomega.Equal(expected.Message))
		g.Expect(cond.ObservedGeneration).To(gomega.Equal(expected.ObservedGeneration))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
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
