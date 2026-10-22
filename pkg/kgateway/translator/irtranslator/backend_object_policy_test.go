package irtranslator

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"
	"time"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils/sessionpersistence"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// namedPolicyIR is a policy IR distinguished by name.
type namedPolicyIR struct{ name string }

func (p *namedPolicyIR) CreationTime() time.Time { return time.Time{} }

func (p *namedPolicyIR) Equals(other any) bool {
	o, ok := other.(*namedPolicyIR)
	return ok && o.name == p.name
}

// recordingBackendObjectPolicyPass records the policies passed to ApplyForBackendObjectPolicies.
type recordingBackendObjectPolicyPass struct {
	ir.UnimplementedProxyTranslationPass
	applied [][]ir.PolicyAtt
	err     error
}

var _ ir.BackendObjectPolicyApplier = &recordingBackendObjectPolicyPass{}

func (p *recordingBackendObjectPolicyPass) ApplyForBackendObjectPolicies(pols []ir.PolicyAtt, pCtx *ir.RouteBackendContext) error {
	p.applied = append(p.applied, pols)
	pCtx.TypedFilterConfig.AddTypedConfig("test.filter", wrapperspb.String("set"))
	return p.err
}

func TestRunBackendObjectPolicies(t *testing.T) {
	gk := schema.GroupKind{Group: "test.kgateway.dev", Kind: "TestPolicy"}
	first, second := &namedPolicyIR{name: "first"}, &namedPolicyIR{name: "second"}
	mergeCalled := false
	merge := func([]ir.PolicyAtt) ir.PolicyAtt { mergeCalled = true; return ir.PolicyAtt{} }

	newTranslator := func(pass ir.ProxyTranslationPass) *httpRouteConfigurationTranslator {
		h := testHTTPRouteTranslator(nil, "")
		h.pluginPass = TranslationPassPlugins{gk: &TranslationPass{ProxyTranslationPass: pass, MergePolicies: merge}}
		return h
	}
	backendWith := func(pols ...ir.PolicyAtt) *ir.BackendObjectIR {
		return &ir.BackendObjectIR{AttachedPolicies: ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{gk: pols}}}
	}

	t.Run("passes the unmerged policies to the applier", func(t *testing.T) {
		mergeCalled = false
		pass := &recordingBackendObjectPolicyPass{}
		pCtx := &ir.RouteBackendContext{}
		pols := []ir.PolicyAtt{{PolicyIr: first}, {PolicyIr: second}}

		require.NoError(t, newTranslator(pass).runBackendObjectPolicies(backendWith(pols...), pCtx))
		require.Len(t, pass.applied, 1, "applier should be called once per policy kind")
		assert.Equal(t, pols, pass.applied[0], "applier should get the unmerged policies")
		assert.False(t, mergeCalled, "the translator should leave merging to the applier")
		assert.NotNil(t, pCtx.TypedFilterConfig.GetTypedConfig("test.filter"), "plugin output should land in the shared typed filter config")
	})

	t.Run("skips plugins that do not implement the applier", func(t *testing.T) {
		mergeCalled = false
		pCtx := &ir.RouteBackendContext{}

		require.NoError(t, newTranslator(ir.UnimplementedProxyTranslationPass{}).runBackendObjectPolicies(backendWith(ir.PolicyAtt{PolicyIr: first}), pCtx))
		assert.False(t, mergeCalled, "policies of plugins without the applier should not be merged")
		assert.Empty(t, pCtx.TypedFilterConfig, "no per-route config should be added")
	})

	t.Run("returns applier errors", func(t *testing.T) {
		pass := &recordingBackendObjectPolicyPass{err: errors.New("boom")}

		err := newTranslator(pass).runBackendObjectPolicies(backendWith(ir.PolicyAtt{PolicyIr: first}), &ir.RouteBackendContext{})
		assert.EqualError(t, err, "boom")
	})

	t.Run("ignores policy kinds without a translation pass", func(t *testing.T) {
		h := testHTTPRouteTranslator(nil, "")

		require.NoError(t, h.runBackendObjectPolicies(backendWith(ir.PolicyAtt{PolicyIr: first}), &ir.RouteBackendContext{}))
	})
}

func TestWarnSessionPersistenceCollisions(t *testing.T) {
	spAny := func(name string) *anypb.Any {
		sp, err := sessionpersistence.Build(sessionpersistence.Config{Type: sessionpersistence.Cookie, Name: name})
		require.NoError(t, err)
		a, err := utils.MessageToAny(sp)
		require.NoError(t, err)
		return a
	}
	cluster := func(name string, sp *anypb.Any) *envoyroutev3.WeightedCluster_ClusterWeight {
		cw := &envoyroutev3.WeightedCluster_ClusterWeight{Name: name}
		if sp != nil {
			cw.TypedPerFilterConfig = map[string]*anypb.Any{sessionpersistence.FilterName: sp}
		}
		return cw
	}
	route := func(routeSP *anypb.Any, clusters ...*envoyroutev3.WeightedCluster_ClusterWeight) *envoyroutev3.Route {
		r := &envoyroutev3.Route{Name: "r", Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
			ClusterSpecifier: &envoyroutev3.RouteAction_WeightedClusters{WeightedClusters: &envoyroutev3.WeightedCluster{Clusters: clusters}},
		}}}
		if routeSP != nil {
			r.TypedPerFilterConfig = map[string]*anypb.Any{sessionpersistence.FilterName: routeSP}
		}
		return r
	}

	tests := []struct {
		name     string
		route    *envoyroutev3.Route
		wantWarn bool
	}{
		{name: "policies with the same name", route: route(nil, cluster("a", spAny("s")), cluster("b", spAny("s"))), wantWarn: true},
		{name: "policies with different names", route: route(nil, cluster("a", spAny("s1")), cluster("b", spAny("s2")))},
		{name: "same backend listed twice", route: route(nil, cluster("a", spAny("s")), cluster("a", spAny("s")))},
		{name: "policy name equals the route rule name", route: route(spAny("s"), cluster("a", spAny("s")), cluster("b", nil)), wantWarn: true},
		{name: "only the route rule config", route: route(spAny("s"), cluster("a", nil), cluster("b", nil))},
		{name: "policy name differs from the route rule name", route: route(spAny("r"), cluster("a", spAny("s")), cluster("b", nil))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := testHTTPRouteTranslator(nil, "")
			h.logger = slog.New(slog.NewTextHandler(&buf, nil))
			h.warnSessionPersistenceCollisions(tt.route)
			if tt.wantWarn {
				assert.Contains(t, buf.String(), "use the same session persistence name", "collision should be logged")
			} else {
				assert.Empty(t, buf.String(), "no collision should be logged")
			}
		})
	}
}
