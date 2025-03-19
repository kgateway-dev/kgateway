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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	envoyadmincli "github.com/kgateway-dev/kgateway/v2/pkg/utils/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
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
		testdefaults.CurlPodManifest,
		simpleServiceManifest,
		gatewayWithRouteManifest,
	}
	s.commonResources = []client.Object{
		// resources from curl manifest
		testdefaults.CurlPod,
		// resources from service manifest
		simpleSvc, simpleDeployment,
		// resources from gateway manifest
		gateway, route, routePolicy,
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
	// make a copy of the original controller deployment
	controllerDeploymentOriginal := &appsv1.Deployment{}
	err := s.testInstallation.ClusterContext.Client.Get(s.ctx, client.ObjectKey{
		Namespace: s.testInstallation.Metadata.InstallNamespace,
		Name:      helpers.DefaultKgatewayDeploymentName,
	}, controllerDeploymentOriginal)
	s.Assert().NoError(err, "has controller deploymnet")

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
