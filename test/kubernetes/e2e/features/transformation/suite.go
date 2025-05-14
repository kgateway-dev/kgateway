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
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	envoyadmincli "github.com/kgateway-dev/kgateway/v2/pkg/utils/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// manifests shared by all tests
	commonManifests []string
	// resources from manifests shared by all tests
	commonResources []client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	s.commonManifests = []string{
		defaults.CurlPodManifest,
		simpleServiceManifest,
		gatewayManifest,
		transformForHeadersManifest,
		transformForBodyJsonManifest,
		transformForBodyAsStringManifest,
		gatewayAttachedTransformManifest,
	}
	s.commonResources = []client.Object{
		// resources from curl manifest
		defaults.CurlPod,
		// resources from service manifest
		simpleSvc, simpleDeployment,
		// resources from gateway manifest
		gateway,
		// resources from specific routes
		routeForHeaders, routeForBodyJson, routeBasic, routeForBodyAsString,
		// deployer-generated resources
		proxyDeployment, proxyService, proxyServiceAccount,
	}

	// set up common resources once
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.commonResources...)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, defaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, simpleDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=backend-0,version=v1",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjectMeta.GetName()),
	})

	s.assertStatus(metav1.Condition{
		Type:    string(gwv1alpha2.PolicyConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  string(gwv1alpha2.PolicyReasonAccepted),
		Message: reports.PolicyAcceptedMsg,
	})
}

func (s *testingSuite) TearDownSuite() {
	// clean up common resources
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err, "can delete "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.commonResources...)

	// make sure pods are gone
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, simpleDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=backend-0,version=v1",
	})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjectMeta.GetName()),
	})
}

func (s *testingSuite) TestGatewayWithTransformedRoute() {
	s.hasDynamicModuleLoaded(false)
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
			name:      "dont pull json info  if not json", // shows we parse the body as json
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
		s.testInstallation.Assertions.AssertEventualCurlResponse(
			s.ctx,
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
	err := s.testInstallation.ClusterContext.Client.Get(s.ctx, client.ObjectKey{
		Namespace: s.testInstallation.Metadata.InstallNamespace,
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
	err = s.testInstallation.ClusterContext.Client.Patch(s.ctx, controllerDeployModified, client.MergeFrom(controllerDeploymentOriginal))
	s.Assert().NoError(err, "patching controller deployment")

	gwDeploymentOriginal := &appsv1.Deployment{}
	err = s.testInstallation.ClusterContext.Client.Get(s.ctx, client.ObjectKey{
		Namespace: "default",
		Name:      "gw",
	}, gwDeploymentOriginal)
	s.Assert().NoError(err, "has gw deploymnet")
	rustBacktraceEnv := corev1.EnvVar{
		Name:  "RUST_BACKTRACE",
		Value: "1",
	}
	gwDeploymentModified := gwDeploymentOriginal.DeepCopy()
	gwDeploymentModified.Spec.Template.Spec.Containers[0].Env = append(
		controllerDeployModified.Spec.Template.Spec.Containers[0].Env,
		rustBacktraceEnv,
	)

	// patch the deployment
	gwDeploymentModified.ResourceVersion = ""
	err = s.testInstallation.ClusterContext.Client.Patch(s.ctx, gwDeploymentModified, client.MergeFrom(gwDeploymentOriginal))
	s.Assert().NoError(err, "patching gateway deployment")

	// wait for the changes to be reflected in pod
	s.testInstallation.Assertions.EventuallyPodContainerContainsEnvVar(
		s.ctx,
		s.testInstallation.Metadata.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=kgateway",
		},
		helpers.KgatewayContainerName,
		rustFormationsEnvVar,
	)

	s.T().Cleanup(func() {
		// revert to original version of deployment
		controllerDeploymentOriginal.ResourceVersion = ""
		err = s.testInstallation.ClusterContext.Client.Patch(s.ctx, controllerDeploymentOriginal, client.MergeFrom(controllerDeployModified))
		s.Require().NoError(err)

		// make sure the env var is removed
		s.testInstallation.Assertions.EventuallyPodContainerDoesNotContainEnvVar(
			s.ctx,
			s.testInstallation.Metadata.InstallNamespace,
			metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/name=kgateway",
			},
			helpers.KgatewayContainerName,
			rustFormationsEnvVar.Name,
		)
	})

	// wait for pods to be running again, since controller deployment was patched
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, s.testInstallation.Metadata.InstallNamespace, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kgateway",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
	})
	s.hasDynamicModuleLoaded(true)

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
		s.testInstallation.Assertions.AssertEventualCurlResponse(
			s.ctx,
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
	p := s.testInstallation.Assertions
	p.Gomega.Eventually(func(g gomega.Gomega) {
		be := &v1alpha1.TrafficPolicy{}
		objKey := client.ObjectKeyFromObject(trafficPolicyForHeaders)
		err := s.testInstallation.ClusterContext.Client.Get(s.ctx, objKey, be)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route policy %s", objKey)
		objKey = client.ObjectKeyFromObject(trafficPolicyForBodyJson)
		err = s.testInstallation.ClusterContext.Client.Get(s.ctx, objKey, be)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route policy %s", objKey)
		objKey = client.ObjectKeyFromObject(trafficPolicyForBodyAsString)
		err = s.testInstallation.ClusterContext.Client.Get(s.ctx, objKey, be)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route policy %s", objKey)
		objKey = client.ObjectKeyFromObject(trafficPolicyForGatewayAttachedTransform)
		err = s.testInstallation.ClusterContext.Client.Get(s.ctx, objKey, be)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route policy %s", objKey)

		actual := be.Status
		g.Expect(actual.Ancestors).To(gomega.HaveLen(1), "should have one ancestor")
		ancestorStatus := actual.Ancestors[0]
		cond := meta.FindStatusCondition(ancestorStatus.Conditions, expected.Type)
		g.Expect(cond).NotTo(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(expected.Status))
		g.Expect(cond.Reason).To(gomega.Equal(expected.Reason))
		g.Expect(cond.Message).To(gomega.Equal(expected.Message))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (s *testingSuite) hasDynamicModuleLoaded(shouldBeLoaded bool) {
	adminClient, closeFwd, err := envoyadmincli.NewPortForwardedClient(s.ctx, "deploy/"+proxyObjectMeta.Name, proxyObjectMeta.Namespace)
	s.Assert().NoError(err, "get admin cli for envoy")

	s.testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		listener, err := adminClient.GetSingleListenerFromDynamicListeners(context.Background(), "http")
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
		WithTimeout(time.Second*20).
		WithPolling(time.Second).Should(gomega.Succeed(), "failed to get expected load of dynamic modules")

	closeFwd()
}
