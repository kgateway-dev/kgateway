//go:build e2e

package basicrouting

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

const (
	kgatewayControllerName = "kgateway.dev/kgateway"
	otherControllerName    = "other-controller.example.com/controller"
)

var (
	// manifests
	serviceManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	headlessServiceManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "headless-service.yaml")
	gatewayWithRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-with-route.yaml")
	routeMissingGwManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-missing-gw.yaml")

	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	// test cases
	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			gatewayWithRouteManifest,
		},
	}
	testCases = map[string]*base.TestCase{
		"TestGatewayWithRoute": {
			Manifests: []string{serviceManifest},
		},
		"TestHeadlessService": {
			Manifests: []string{headlessServiceManifest},
		},
		"TestClearStaleStatus": {
			Manifests: []string{serviceManifest},
		},
	}

	listenerHighPort = 8080
	listenerLowPort  = 80
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

func (s *testingSuite) TestGatewayWithRoute() {
	s.assertSuccessfulResponse()
}

func (s *testingSuite) TestHeadlessService() {
	s.assertSuccessfulResponse()
}

func (s *testingSuite) TestClearStaleStatus() {
	// Inject fake parent status from another controller
	s.addParentStatus("example-route", "default", otherControllerName)

	// Verify status
	s.assertParentStatuses("gw", map[string]bool{
		kgatewayControllerName: true,
		otherControllerName:    true,
	})

	// Modify route to reference missing-gw
	err := s.TestInstallation.Actions.Kubectl().ApplyFile(
		s.Ctx,
		routeMissingGwManifest,
	)
	s.Require().NoError(err)

	// Verify kgateway status is cleared but other controller status remains
	s.assertParentStatuses("gw", map[string]bool{
		kgatewayControllerName: false,
		otherControllerName:    true,
	})

	// Re-apply the correct parent ref
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(
		s.Ctx,
		gatewayWithRouteManifest,
	)
	s.Require().NoError(err)
}

func (s *testingSuite) assertSuccessfulResponse() {
	for _, port := range []int{listenerHighPort, listenerLowPort} {
		s.TestInstallation.Assertions.AssertEventualCurlResponse(
			s.Ctx,
			testdefaults.CurlPodExecOpt,
			[]curl.Option{
				curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
				curl.WithHostHeader("example.com"),
				curl.WithPort(port),
			},
			&testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Body:       gomega.ContainSubstring(testdefaults.NginxResponse),
			})
	}
}

func (s *testingSuite) addParentStatus(routeName, routeNamespace, controllerName string) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
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
				Name: "gw",
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
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
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
