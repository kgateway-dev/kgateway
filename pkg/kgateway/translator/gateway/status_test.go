package gateway_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	translatortest "github.com/kgateway-dev/kgateway/v2/test/translator"
)

func TestStatuses(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		dir := fsutils.MustGetThisDir()
		settingOpt := func(s *apisettings.Settings) {
			s.EnableExperimentalGatewayAPIFeatures = true
			s.EnableAuthMetadata = true
		}
		translatortest.TestTranslation(
			t,
			t.Context(),
			[]string{
				filepath.Join(dir, "testutils/inputs/status/basic.yaml"),
			},
			filepath.Join(dir, "testutils/outputs/status/basic.yaml"),
			types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			settingOpt,
		)
	})
}

// TestRouteRejectionConditionHasMessage verifies that when an HTTPRoute is rejected
// (e.g. its sectionName doesn't match any listener), the Accepted=False condition
// includes a non-empty message so operators can diagnose the problem via kubectl.
func TestRouteRejectionConditionHasMessage(t *testing.T) {
	dir := fsutils.MustGetThisDir()
	gwNN := types.NamespacedName{Namespace: "default", Name: "example-gateway"}

	tc := translatortest.TestCase{
		InputFiles: []string{
			filepath.Join(dir, "testutils/inputs/status/route-rejection.yaml"),
		},
	}
	scheme := translatortest.NewScheme(translatortest.ExtraConfig{}.Schemes)
	results, err := tc.Run(t, context.Background(), scheme, translatortest.ExtraConfig{})
	require.NoError(t, err)
	require.Contains(t, results, gwNN)

	reportsMap := results[gwNN].ReportsMap
	routeNN := types.NamespacedName{Namespace: "default", Name: "rejected-route"}
	routeReport := reportsMap.HTTPRoutes[routeNN]
	require.NotNil(t, routeReport, "expected a route report for rejected-route")

	// Find any Accepted=False condition across all parent refs and assert it has a message.
	var acceptedCond *metav1.Condition
	for _, parentReport := range routeReport.Parents {
		for i := range parentReport.Conditions {
			c := &parentReport.Conditions[i]
			if c.Type == string(gwv1.RouteConditionAccepted) && c.Status == metav1.ConditionFalse {
				acceptedCond = c
				break
			}
		}
	}
	require.NotNil(t, acceptedCond, "expected an Accepted=False condition for the rejected route")
	assert.NotEmpty(t, acceptedCond.Message, "Accepted=False condition must include a message explaining why the route was rejected")
}
