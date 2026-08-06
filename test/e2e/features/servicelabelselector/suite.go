//go:build e2e

package servicelabelselector

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

const (
	serviceLabelSelector = "e2e.kgateway.dev/discovery=enabled,e2e.kgateway.dev/excluded!=true"
	gatewayName          = "service-label-selector"
)

var (
	setupManifest                   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	matchingServiceManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "matching-service.yaml")
	positiveExcludedServiceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "positive-excluded-service.yaml")
	negativeExcludedServiceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "negative-excluded-service.yaml")

	setup = base.TestCase{
		Manifests: []string{setupManifest},
	}
	testCases = map[string]*base.TestCase{
		"TestPositiveAndNegativeSelectorsIncludeMatchingService": {
			Manifests: []string{matchingServiceManifest},
		},
		"TestPositiveSelectorExcludesNonMatchingService": {
			Manifests: []string{positiveExcludedServiceManifest},
		},
		"TestNegativeSelectorExcludesMatchingService": {
			Manifests: []string{negativeExcludedServiceManifest},
		},
	}
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
	gateway common.Gateway
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()
	s.gateway = common.Gateway{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: gatewayName},
		Address: s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayAddress(
			s.Ctx,
			gatewayName,
			"default",
		),
	}
}

func (s *testingSuite) TestPositiveAndNegativeSelectorsIncludeMatchingService() {
	s.assertResolvedRefs(
		"matching-service-route",
		metav1.ConditionTrue,
		string(gwv1.RouteReasonResolvedRefs),
	)
	s.gateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(testdefaults.NginxResponse),
		},
		curl.WithHostHeader("matching-selector.example.com"),
		curl.WithPort(8080),
	)
}

func (s *testingSuite) TestPositiveSelectorExcludesNonMatchingService() {
	s.assertResolvedRefs(
		"positive-excluded-service-route",
		metav1.ConditionFalse,
		string(gwv1.RouteReasonBackendNotFound),
		"Service default/positive-excluded-service not found",
		serviceLabelSelector,
		"controller.discovery.serviceLabelSelector",
		"KGW_SERVICE_LABEL_SELECTOR",
		"verify the Service labels match the selector",
	)
}

func (s *testingSuite) TestNegativeSelectorExcludesMatchingService() {
	s.assertResolvedRefs(
		"negative-excluded-service-route",
		metav1.ConditionFalse,
		string(gwv1.RouteReasonBackendNotFound),
		"Service default/negative-excluded-service not found",
		serviceLabelSelector,
		"controller.discovery.serviceLabelSelector",
		"KGW_SERVICE_LABEL_SELECTOR",
		"verify the Service labels match the selector",
	)
}

func (s *testingSuite) assertResolvedRefs(
	routeName string,
	expectedStatus metav1.ConditionStatus,
	expectedReason string,
	expectedMessageSubstrings ...string,
) {
	s.T().Helper()
	timeout, pollingInterval := helpers.GetTimeouts()

	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			client.ObjectKey{Namespace: "default", Name: routeName},
			route,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get HTTPRoute default/%s", routeName)

		var resolvedRefs *metav1.Condition
		for parentIndex := range route.Status.Parents {
			parent := &route.Status.Parents[parentIndex]
			if parent.ParentRef.Name != gwv1.ObjectName(gatewayName) {
				continue
			}
			for conditionIndex := range parent.Conditions {
				condition := &parent.Conditions[conditionIndex]
				if condition.Type != string(gwv1.RouteConditionResolvedRefs) {
					continue
				}
				resolvedRefs = condition
				break
			}
		}

		g.Expect(resolvedRefs).NotTo(
			gomega.BeNil(),
			"ResolvedRefs condition for Gateway %s was not found; full route status: %+v",
			gatewayName,
			route.Status,
		)
		if resolvedRefs == nil {
			return
		}

		g.Expect(resolvedRefs.Status).To(gomega.Equal(expectedStatus), "full route status: %+v", route.Status)
		g.Expect(resolvedRefs.Reason).To(gomega.Equal(expectedReason), "full route status: %+v", route.Status)
		for _, substring := range expectedMessageSubstrings {
			g.Expect(resolvedRefs.Message).To(gomega.ContainSubstring(substring), "full route status: %+v", route.Status)
		}
	}, timeout, pollingInterval).Should(gomega.Succeed())
}
