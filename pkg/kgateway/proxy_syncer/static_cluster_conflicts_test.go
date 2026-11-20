package proxy_syncer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestStaticClusterConflicts(t *testing.T) {
	// Index two Service backends by their translated cluster names.
	httpbin := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "default", Name: "httpbin"}, 8000, "", "kube")
	echo := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "default", Name: "echo"}, 80, "", "kube")
	backends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{&httpbin, &echo})
	conflicts := newStaticClusterConflicts(backends)

	// report returns the StaticClusterConflicted condition it sets on a Gateway whose
	// patched bootstrap has staticClusters, or nil when it sets none.
	report := func(staticClusters ...string) *metav1.Condition {
		gw := ir.Gateway{Obj: &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 2}}}
		if staticClusters != nil {
			gw.ValidationBootstraps = &ir.GatewayValidationBootstraps{StaticClusters: staticClusters}
		}
		rm := reports.NewReportMap()
		conflicts.report(krt.TestingDummyContext{}, gw, &rm)
		gwReport := rm.Gateway(gw.Obj)
		if gwReport == nil {
			return nil
		}
		return meta.FindStatusCondition(gwReport.GetConditions(), conditions.KgatewayConditionStaticClusterConflicted)
	}

	t.Run("a Gateway without bootstrap patches gets no condition", func(t *testing.T) {
		// Check a Gateway with no static cluster names.
		assert.Nil(t, report(), "a Gateway without bootstrap patches should get no condition")
	})

	t.Run("no conflicts", func(t *testing.T) {
		// Check static clusters that no translated cluster is named after.
		cond := report("admin_port_cluster", "xds_cluster")

		// The condition says there are none.
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, conditions.KgatewayReasonNoStaticClusterConflicts, cond.Reason)
	})

	t.Run("a static cluster shadows a translated cluster", func(t *testing.T) {
		// Check static clusters, one with the name of httpbin's translated cluster.
		cond := report("xds_cluster", httpbin.ClusterName())

		// The condition names the static cluster and the backend, and not the other one.
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, conditions.KgatewayReasonTranslatedClusterShadowed, cond.Reason)
		assert.Contains(t, cond.Message, `static cluster "`+httpbin.ClusterName()+`" shadows the cluster for Service default/httpbin port 8000`)
		assert.NotContains(t, cond.Message, "echo")
	})
}
