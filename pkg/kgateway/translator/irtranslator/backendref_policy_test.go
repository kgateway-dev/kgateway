package irtranslator

import (
	"context"
	"errors"
	"testing"
	"time"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// clusterSpecifierBackendPass is a backend kind's translation pass that sets the route's
// cluster specifier, like a plugin that picks the cluster itself.
type clusterSpecifierBackendPass struct {
	ir.UnimplementedProxyTranslationPass
}

func (p *clusterSpecifierBackendPass) ApplyForBackend(_ *ir.RouteBackendContext, _ ir.HttpBackend, out *envoyroutev3.Route) error {
	out.GetRoute().ClusterSpecifier = &envoyroutev3.RouteAction_Cluster{Cluster: "plugin-cluster"}
	return nil
}

// weightedRouteWithFailingBackend returns a route to a healthy backend and to a backend whose
// backendRef filters failed to resolve.
func weightedRouteWithFailingBackend() ir.HttpRouteRuleMatchIR {
	in := testRouteIR(0, "/", "healthy-cluster")
	in.Backends = append(in.Backends, ir.HttpBackend{
		Backend:    ir.BackendRefIR{ClusterName: "failing-cluster", Weight: 1},
		FiltersErr: krtcollections.ErrPolicyNotFound,
	})
	return in
}

func TestEnvoyRoutesFailsClosedWhenPluginSetsClusterSpecifier(t *testing.T) {
	backendGK := schema.GroupKind{Group: "test.kgateway.dev", Kind: "TestBackend"}
	h := testHTTPRouteTranslator(nil, apisettings.ValidationStandard)
	h.pluginPass = TranslationPassPlugins{backendGK: &TranslationPass{ProxyTranslationPass: &clusterSpecifierBackendPass{}}}
	report := &reports.ParentRefReport{}

	in := weightedRouteWithFailingBackend()
	in.Backends[0].Backend.BackendObject = new(ir.NewBackendObjectIR(ir.ObjectSource{Group: backendGK.Group, Kind: backendGK.Kind, Name: "be"}, 0, "", ""))

	out := h.envoyRoutes(report, in, "test-route")

	require.NotNil(t, out, "the route should be replaced, not dropped")
	assert.Equal(t, uint32(500), out.GetDirectResponse().GetStatus(),
		"the plugin's cluster specifier would bypass the failing backend's blackhole cluster, so the route should fail closed")
	cond := findProgrammedCondition(report.Conditions)
	require.NotNil(t, cond, "the route should report the error")
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, reportssdk.RouteRuleReplacedReason, cond.Reason)
}

func TestEnvoyRoutesCountsPartialReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h := testHTTPRouteTranslator(nil, apisettings.ValidationStandard)
	h.gw.SourceObject.ObjectSource = ir.ObjectSource{Name: "gw-a", Namespace: "ns-a"}
	report := &reports.ParentRefReport{}
	routeReplacementsTotal.Reset()

	out := h.envoyRoutes(report, weightedRouteWithFailingBackend(), "test-route")

	require.NotNil(t, out.GetRoute(), "the healthy backend should keep its traffic")
	cond := findProgrammedCondition(report.Conditions)
	require.NotNil(t, cond, "the route should report the error")
	assert.Equal(t, reportssdk.BackendRefReplacedReason, cond.Reason)
	gathered := metricstest.MustGatherMetricsContext(ctx, t, "kgateway_routing_replacements_total")
	assert.Equal(t, float64(1), gathered.MustGetMetricValueByLabels(
		"kgateway_routing_replacements_total",
		[]metrics.Label{
			{Name: gatewayNamespaceLabel, Value: "ns-a"},
			{Name: gatewayLabel, Value: "gw-a"},
			{Name: errorTypeLabel, Value: ErrTypeRefNotFound},
		},
	), "a partial replacement should be counted")
}

func TestReceivesTraffic(t *testing.T) {
	assert.True(t, receivesTraffic(ir.BackendRefIR{ClusterName: "c", Weight: 1}))
	assert.False(t, receivesTraffic(ir.BackendRefIR{ClusterName: "c", Weight: 0}), "zero weight")
	assert.False(t, receivesTraffic(ir.BackendRefIR{ClusterName: wellknown.BlackholeClusterName, Weight: 1}), "unresolved backendRef")
	assert.False(t, receivesTraffic(ir.BackendRefIR{ClusterName: "c", Weight: 1, Err: errors.New("invalid")}), "invalid backendRef")
}

func findProgrammedCondition(conds []metav1.Condition) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == conditions.KgatewayConditionProgrammed {
			return &conds[i]
		}
	}
	return nil
}
