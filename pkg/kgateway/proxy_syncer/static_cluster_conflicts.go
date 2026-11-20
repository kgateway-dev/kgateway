package proxy_syncer

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// staticClusterConflicts finds translated backend clusters that a Gateway's bootstrap
// shadows. Envoy does not let CDS update a cluster defined in the bootstrap: it skips
// the CDS entry and logs it only at debug level. So when a static cluster from
// bootstrap patches has the name of a translated cluster, traffic to that backend
// uses the static cluster, and changes to the backend never reach the proxy.
type staticClusterConflicts struct {
	backends      krt.Collection[*ir.BackendObjectIR]
	byClusterName krt.Index[string, *ir.BackendObjectIR]
}

// newStaticClusterConflicts indexes backends by the name of the cluster translated for
// each, so a Gateway depends only on the backends whose cluster name one of its static
// clusters has.
func newStaticClusterConflicts(backends krt.Collection[*ir.BackendObjectIR]) staticClusterConflicts {
	return staticClusterConflicts{
		backends: backends,
		byClusterName: krtpkg.UnnamedIndex(backends, func(b *ir.BackendObjectIR) []string {
			return []string{b.ClusterName()}
		}),
	}
}

// report sets the kgateway.dev/StaticClusterConflicted condition on gw in rm. A
// backend is reported whether or not a route uses it yet. A Gateway without bootstrap
// patches gets no condition.
func (s staticClusterConflicts) report(kctx krt.HandlerContext, gw ir.Gateway, rm *reports.ReportMap) {
	// Only a Gateway with bootstrap patches has static cluster names to check.
	names := gw.ValidationBootstraps.GetStaticClusters()
	if len(names) == 0 {
		return
	}

	// Find each backend whose translated cluster has a static cluster's name.
	var conflicts []string
	for _, name := range names {
		matches := krt.Fetch(kctx, s.backends, krt.FilterIndex(s.byClusterName, name))
		slices.SortFunc(matches, func(a, b *ir.BackendObjectIR) int { return cmp.Compare(a.ResourceName(), b.ResourceName()) })
		for _, b := range matches {
			conflicts = append(conflicts, fmt.Sprintf("static cluster %q shadows the cluster for %s %s/%s port %d",
				name, b.GetGroupKind().Kind, b.GetNamespace(), b.GetName(), b.GetPort()))
		}
	}

	// Report the conflicts, or that there are none.
	cond := reporter.GatewayCondition{
		Type:    gwv1.GatewayConditionType(conditions.KgatewayConditionStaticClusterConflicted),
		Status:  metav1.ConditionFalse,
		Reason:  gwv1.GatewayConditionReason(conditions.KgatewayReasonNoStaticClusterConflicts),
		Message: "No static cluster in the bootstrap has the name of a cluster translated for a backend.",
	}
	if len(conflicts) > 0 {
		cond.Status = metav1.ConditionTrue
		cond.Reason = gwv1.GatewayConditionReason(conditions.KgatewayReasonTranslatedClusterShadowed)
		cond.Message = "Envoy ignores the cluster translated for each of these backends because a static cluster in the bootstrap overlay has its name, " +
			"so traffic to the backend goes through the static cluster and changes to the backend do not reach the proxy; " +
			"rename the static cluster in the GatewayParameters bootstrap overlay: " + strings.Join(conflicts, "; ") + "."
	}
	reports.NewReporter(rm).Gateway(gw.Obj).SetCondition(cond)
}
