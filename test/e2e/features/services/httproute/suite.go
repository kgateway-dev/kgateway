//go:build e2e

package httproute

import (
	"context"
	"fmt"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// testingSuite is the entire Suite of tests for testing K8s Service-specific features/fixes
type testingSuite struct {
	*base.BaseTestingSuite
}

var (
	// No setup since we handle manifests manually in each test
	setup = base.TestCase{}

	testCases = map[string]*base.TestCase{
		"TestConfigureHTTPRouteBackingDestinationsWithService": {},
		// If the TCPRoute CRD is not installed, TestConfigureHTTPRouteBackingDestinationsWithService implicitly tests that HTTPRoute services still work without the CRD
		"TestConfigureHTTPRouteBackingDestinationsWithServiceAndWithoutTCPRoute": {
			MinGwApiVersion: base.GwApiRequireTcpRoutes,
		},
		"TestClearStaleStatus": {},
	}
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) TestConfigureHTTPRouteBackingDestinationsWithService() {
	testutils.Cleanup(s.T(), func() {
		err := s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, routeWithServiceManifest)
		s.NoError(err, "can delete manifest")
		err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, serviceManifest)
		s.NoError(err, "can delete manifest")
		s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx, proxyService, proxyDeployment)
	})

	err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, routeWithServiceManifest)
	s.Assert().NoError(err, "can apply manifest")

	// apply the service manifest separately, after the route table is applied, to ensure it can be applied after the route table
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, serviceManifest)
	s.Assert().NoError(err, "can apply manifest")

	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.Ctx, proxyService, proxyDeployment)
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, nginxMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=nginx",
	})
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=gw",
	})

	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedSvcResp)
}

func (s *testingSuite) TestConfigureHTTPRouteBackingDestinationsWithServiceAndWithoutTCPRoute() {
	// Get the current TCPRoute CRD before deleting it. We don't know until runtime which version is installed.
	tcpRouteCrdYaml, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "get", "crd", "tcproutes.gateway.networking.k8s.io", "-o", "yaml")
	s.Assert().NoError(err, "can get TCPRoute CRD")

	testutils.Cleanup(s.T(), func() {
		err := s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, routeWithServiceManifest)
		s.Assert().NoError(err, "can delete manifest")
		err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, serviceManifest)
		s.Assert().NoError(err, "can delete manifest")
		s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx, proxyService, proxyDeployment)

		// Restore the TCPRoute CRD using the saved content
		err = s.TestInstallation.Actions.Kubectl().Create(s.Ctx, []byte(tcpRouteCrdYaml))
		s.NoError(err, "can apply TCPRoute CRD")
		s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.Ctx, &wellknown.TCPRouteCRD)
	})

	// Remove the TCPRoute CRD to assert HTTPRoute services still work.
	err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, tcpRouteCrdManifest)
	s.Assert().NoError(err, "can delete manifest")

	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, routeWithServiceManifest)
	s.Assert().NoError(err, "can apply manifest")

	// apply the service manifest separately, after the route table is applied, to ensure it can be applied after the route table
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, serviceManifest)
	s.Assert().NoError(err, "can apply manifest")

	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.Ctx, proxyService, proxyDeployment)
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, nginxMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=nginx",
	})
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=gw",
	})

	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedSvcResp)
}

func (s *testingSuite) TestClearStaleStatus() {
	testutils.Cleanup(s.T(), func() {
		// routeMissingGwManifest only modify the route thus cleaning up routeWithServiceManifest is enough.
		err := s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, routeWithGwManifest)
		s.NoError(err, "can delete manifest")
		s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx, proxyService, proxyDeployment)
	})

	err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, routeWithGwManifest)
	s.Assert().NoError(err, "can apply manifest")

	// Inject fake parent status from another controller
	s.addParentStatus("example-route", "default", "other-gw", otherControllerName)

	// Verify status
	s.assertParentStatuses("gw", map[string]bool{
		kgatewayControllerName: true,
	})
	s.assertParentStatuses("other-gw", map[string]bool{
		otherControllerName: true,
	})

	// Modify route to reference missing-gw
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, routeMissingGwManifest)
	s.Require().NoError(err, "can apply manifest")

	// Verify kgateway status is cleared but other controller status remains
	s.assertParentStatuses("gw", map[string]bool{
		kgatewayControllerName: false,
	})
	s.assertParentStatuses("other-gw", map[string]bool{
		otherControllerName: true,
	})
}

func (s *testingSuite) addParentStatus(routeName, routeNamespace, gwName, controllerName string) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			types.NamespacedName{Name: routeName, Namespace: routeNamespace},
			route,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		// Add fake parent status entry
		fakeStatus := gwv1.RouteParentStatus{
			ParentRef: gwv1.ParentReference{
				Name: gwv1.ObjectName(gwName),
			},
			ControllerName: gwv1.GatewayController(controllerName),
			Conditions: []metav1.Condition{
				{
					Type:               string(gwv1.RouteConditionAccepted),
					Status:             metav1.ConditionTrue,
					Reason:             string(gwv1.RouteReasonAccepted),
					Message:            "Accepted by fake controller",
					LastTransitionTime: metav1.Now(),
				},
			},
		}

		route.Status.Parents = append(route.Status.Parents, fakeStatus)
		err = s.TestInstallation.ClusterContext.Client.Status().Update(s.Ctx, route)
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (s *testingSuite) assertParentStatuses(parentName string, expectedControllers map[string]bool) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			types.NamespacedName{Name: "example-route", Namespace: "default"},
			route,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get HTTPRoute")

		// Build map of found controllers for this parent
		foundControllers := make(map[string]bool)
		for _, parent := range route.Status.Parents {
			if string(parent.ParentRef.Name) == parentName {
				foundControllers[string(parent.ControllerName)] = true
			}
		}

		// Verify each expected controller status
		for controller, shouldExist := range expectedControllers {
			exists := foundControllers[controller]
			if shouldExist {
				g.Expect(exists).To(gomega.BeTrue(),
					fmt.Sprintf("parent status for gateway %s with controller %s should exist. Full status: %+v",
						parentName, controller, route.Status))
			} else {
				g.Expect(exists).To(gomega.BeFalse(),
					fmt.Sprintf("parent status for gateway %s with controller %s should not exist. Full status: %+v",
						parentName, controller, route.Status))
			}
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}
