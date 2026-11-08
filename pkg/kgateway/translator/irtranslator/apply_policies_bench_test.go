package irtranslator

import (
	"fmt"
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type routePassFunc struct {
	ir.UnimplementedProxyTranslationPass
}

func (routePassFunc) ApplyForRoute(*ir.RouteContext, *envoyroutev3.Route) error { return nil }

// BenchmarkRunRoutePlugins measures the per-route policy driver: three group
// kinds, one of them merged, each with a policy that reports status.
func BenchmarkRunRoutePlugins(b *testing.B) {
	h := testHTTPRouteTranslator(nil, "")
	rm := reports.NewReportMap()
	h.reporter = reports.NewReporter(&rm)
	var in ir.HttpRouteRuleMatchIR
	in.AttachedPolicies.Policies = map[schema.GroupKind][]ir.PolicyAtt{}
	for i := range 3 {
		gk := schema.GroupKind{Group: "test", Kind: fmt.Sprintf("Policy%d", i)}
		pass := &TranslationPass{ProxyTranslationPass: routePassFunc{}}
		if i == 0 {
			pass.MergePolicies = func(pols []ir.PolicyAtt) ir.PolicyAtt { return pols[0] }
		}
		h.pluginPass[gk] = pass
		in.AttachedPolicies.Policies[gk] = []ir.PolicyAtt{{
			GroupKind: gk,
			PolicyRef: &ir.AttachedPolicyRef{Group: gk.Group, Kind: gk.Kind, Namespace: "default", Name: "p"},
		}}
	}
	out := &envoyroutev3.Route{}
	typedPerFilterConfig := ir.TypedFilterConfigMap{}
	b.ReportAllocs()
	for b.Loop() {
		if err := h.runRoutePlugins(in, out, typedPerFilterConfig); err != nil {
			b.Fatal(err)
		}
	}
}
