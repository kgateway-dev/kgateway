package transformation

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"strings"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"

	envoyadmincli "github.com/kgateway-dev/kgateway/v2/pkg/utils/envoyutils/admincli"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) TestGatewayWithTransformedRoute() {
	manifests := []string{
		testdefaults.CurlPodManifest,
		simpleServiceManifest,
		gatewayWithRouteManifest,
	}
	manifestObjects := []client.Object{
		testdefaults.CurlPod,                               // curl
		simpleSvc,                                          // echo service
		proxyService, proxyServiceAccount, proxyDeployment, // proxy
	}

	s.T().Cleanup(func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, manifestObjects...)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, manifestObjects...)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})

	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
	})

	testCasess := []struct {
		name string
		opts []curl.Option
		resp *testmatchers.HttpResponse
	}{
		{
			name: "basic",
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
			name: "conditional set by request header", // inja and the request_header function in use
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
	for _, tc := range testCasess {
		s.testInstallation.Assertions.AssertEventualCurlResponse(
			s.ctx,
			testdefaults.CurlPodExecOpt,
			append(tc.opts,
				curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
				curl.WithHostHeader("example.com"),
				curl.WithPort(8080),
			),
			tc.resp)
	}
}

func (s *testingSuite) TestGatewayRustformationsWithTransformedRoute() {
	manifests := []string{
		testdefaults.CurlPodManifest,
		simpleServiceManifest,
		gatewayWithRouteManifest,
	}
	manifestObjects := []client.Object{
		testdefaults.CurlPod,                               // curl
		simpleSvc,                                          // echo service
		proxyService, proxyServiceAccount, proxyDeployment, // proxy
	}

	controllerDeploymentOriginal := &appsv1.Deployment{}
	err := s.testInstallation.ClusterContext.Client.Get(s.ctx, client.ObjectKey{
		Namespace: s.testInstallation.Metadata.InstallNamespace,
		Name:      "kgateway",
	}, controllerDeploymentOriginal)
	s.Assert().NoError(err, "has controller deploymnet")

	controllerDeploy := controllerDeploymentOriginal.DeepCopy()
	// add the environment variable RUSTFORMATIONS to the controller deployment

	env := append(controllerDeploy.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  "KGW_USE_RUST_FORMATIONS",
		Value: "true",
	})
	containers := controllerDeploy.Spec.Template.Spec.Containers
	containers[0].Env = env
	controllerDeploy.Spec.Template.Spec.Containers = containers

	// patch the actual deployment with the new environment variable
	err = s.testInstallation.ClusterContext.Client.Patch(s.ctx, controllerDeploy, client.MergeFrom(controllerDeploymentOriginal))
	s.Assert().NoError(err, "patching controller deployment")

	s.T().Cleanup(func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, manifestObjects...)
		err = s.testInstallation.ClusterContext.Client.Patch(s.ctx, controllerDeploy, client.MergeFrom(controllerDeploy))
		s.Require().NoError(err)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, manifestObjects...)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})

	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
	})

	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, s.testInstallation.Metadata.InstallNamespace, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kgateway",
	})

	adminClient, closeFwd, err := envoyadmincli.NewPortForwardedClient(s.ctx, "deploy/"+proxyObjectMeta.Name, proxyObjectMeta.Namespace)
	s.Assert().NoError(err, "get admin cli for envoy")

	s.testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		listener, err := adminClient.GetSingleListenerFromDynamicListeners(context.Background(), "http")
		g.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get listener")

		// use a weak filter name check for cyclic imports
		// also we dont intend for this to be long term so dont worry about pulling it out to wellknown or something like that for now
		dynamicModuleLoaded := strings.Contains(listener.String(), "dynamic_modules/")
		g.Expect(dynamicModuleLoaded).To(gomega.BeTrue(), fmt.Sprintf("dynamic module not loaded: %v", listener.String()))
		dynamicModuleRouteConfigured := strings.Contains(listener.String(), "transformation/helper")
		g.Expect(dynamicModuleRouteConfigured).To(gomega.BeTrue(), fmt.Sprintf("dynamic module routespecific not loaded: %v", listener.String()))
	}).
		WithTimeout(time.Second*20).
		WithPolling(time.Second).Should(gomega.Succeed(), "failed to load in dynamic modules")

	closeFwd()

	testCasess := []struct {
		name string
		opts []curl.Option
		resp *testmatchers.HttpResponse
	}{
		{
			name: "basic",
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
			name: "conditional set by request header", // inja and the request_header function in use
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
	for _, tc := range testCasess {
		s.testInstallation.Assertions.AssertEventualCurlResponse(
			s.ctx,
			testdefaults.CurlPodExecOpt,
			append(tc.opts,
				curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
				curl.WithHostHeader("example.com"),
				curl.WithPort(8080),
			),
			tc.resp)
	}
}
