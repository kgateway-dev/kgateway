package translator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// AssertAcceptedPolicyStatus is a helper function to verify policy status conditions
func AssertAcceptedPolicyStatus(t *testing.T, reportsMap reports.ReportMap, policies []reports.PolicyKey) {
	AssertPolicyStatusWithGeneration(t, reportsMap, policies, 0)
}

// AssertPolicyStatusWithGeneration is a helper function to verify policy status conditions with a specific generation
func AssertPolicyStatusWithGeneration(t *testing.T, reportsMap reports.ReportMap, policies []reports.PolicyKey, expectedGeneration int64) {
	var currentStatus gwv1alpha2.PolicyStatus

	a := assert.New(t)
	for _, policy := range policies {
		// Validate each policy's status
		status := reportsMap.BuildPolicyStatus(context.Background(), policy, wellknown.DefaultGatewayControllerName, currentStatus)
		a.NotNilf(status, "status missing for policy %v", policy)
		a.Len(status.Ancestors, 1, "ancestor missing for policy %v", policy) // 1 Gateway(ancestor)

		acceptedCondition := meta.FindStatusCondition(status.Ancestors[0].Conditions, string(v1alpha1.PolicyConditionAccepted))
		a.NotNilf(acceptedCondition, "Accepted condition missing for policy %v", policy)
		a.Equalf(metav1.ConditionTrue, acceptedCondition.Status, "Accepted condition Status mismatch for policy %v", policy)
		a.Equalf(string(v1alpha1.PolicyReasonValid), acceptedCondition.Reason, "Accepted condition Reason mismatch for policy %v", policy)
		a.Equalf(reporter.PolicyAcceptedMsg, acceptedCondition.Message, "Accepted condition Message mismatch for policy %v", policy)
		a.Equalf(expectedGeneration, acceptedCondition.ObservedGeneration, "Accepted condition ObservedGeneration mismatch for policy %v", policy)
	}
}

// AssertRouteInvalidDropped is a helper for asserting that a route has the Accepted=false status condition
// for dropped rules with variadic expected message substrings.
func AssertRouteInvalidDropped(t *testing.T, routeName, namespace string, expectedMsgSubstrings ...string) AssertReports {
	return func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
		a := assert.New(t)
		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeName,
				Namespace: namespace,
			},
		}
		routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
		a.NotNil(routeStatus, "Route status should not be nil")
		a.Len(routeStatus.Parents, 1, "Route should have one parent")

		resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
		a.NotNil(resolvedRefs, "ResolvedRefs condition should not be nil")
		a.Equal(metav1.ConditionTrue, resolvedRefs.Status, "ResolvedRefs Status mismatch")
		a.Equal(string(gwv1.RouteReasonResolvedRefs), resolvedRefs.Reason, "ResolvedRefs Reason mismatch")
		a.NotEmpty(resolvedRefs.Message, "ResolvedRefs Message should not be empty")

		accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
		a.NotNil(accepted, "Accepted condition should not be nil")
		a.Equal(metav1.ConditionFalse, accepted.Status, "Accepted Status mismatch")
		a.Equal(string(reporter.RouteRuleDroppedReason), accepted.Reason, "Accepted Reason mismatch")
		for _, msgSubstring := range expectedMsgSubstrings {
			a.Contains(accepted.Message, msgSubstring, "Accepted Message mismatch")
		}
		a.Equal(int64(0), accepted.ObservedGeneration, "Accepted ObservedGeneration mismatch")
	}
}
