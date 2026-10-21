package irtranslator

import (
	"errors"
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestRouteCacheKeepsOnlyWhatTheLastTranslationUsed(t *testing.T) {
	var c routeCache
	gw := types.NamespacedName{Namespace: "ns", Name: "gw"}
	k1 := routeKey{route: &ir.HttpRouteIR{}, generatedName: "vh-route-0"}
	k2 := routeKey{route: &ir.HttpRouteIR{}, generatedName: "vh-route-1"}
	r1 := &envoyroutev3.Route{Name: "r1"}

	translate := func(rcName string, use func(*routeCacheGeneration)) {
		c.beginGateway(gw)
		gen := c.begin(gw, rcName)
		use(gen)
		c.end(gen)
		c.endGateway(gw)
	}

	translate("rc", func(gen *routeCacheGeneration) {
		require.Nil(t, gen.get(k1, true), "cache should start empty")
		require.Same(t, r1, gen.put(k1, r1, true), "a shared route should be used as is")
	})
	translate("rc", func(gen *routeCacheGeneration) {
		require.Same(t, r1, gen.get(k1, true), "a shared route should be replayed as is")
		require.Nil(t, gen.get(k2, true), "an uncached route should miss")
	})
	translate("rc", func(*routeCacheGeneration) {})
	translate("rc", func(gen *routeCacheGeneration) {
		require.Nil(t, gen.get(k1, true), "a route the last translation did not use should be dropped")
		gen.put(k1, r1, true)
	})
	translate("other", func(*routeCacheGeneration) {})
	translate("rc", func(gen *routeCacheGeneration) {
		require.Nil(t, gen.get(k1, true), "a route configuration the last translation did not produce should be dropped")
		gen.put(k1, r1, true)
	})

	c.forgetGateway(gw)
	translate("rc", func(gen *routeCacheGeneration) {
		require.Nil(t, gen.get(k1, true), "a forgotten Gateway should have nothing cached")
	})

	require.Nil(t, c.begin(gw, "rc"), "the cache should be off outside a Gateway translation")
}

func TestRouteCacheCopiesRoutesThatMayBeEdited(t *testing.T) {
	var c routeCache
	gw := types.NamespacedName{Namespace: "ns", Name: "gw"}
	key := routeKey{route: &ir.HttpRouteIR{}}
	route := &envoyroutev3.Route{Name: "r"}

	c.beginGateway(gw)
	gen := c.begin(gw, "rc")
	used := gen.put(key, route, false)
	require.NotSame(t, route, used, "an unshared route should be a copy")
	used.Name = "edited"
	c.end(gen)
	c.endGateway(gw)

	c.beginGateway(gw)
	gen = c.begin(gw, "rc")
	replayed := gen.get(key, false)
	require.NotSame(t, route, replayed, "an unshared replay should be a copy")
	require.True(t, proto.Equal(replayed, &envoyroutev3.Route{Name: "r"}), "edits to a copy should not reach the cache")
}

func TestRouteCacheKeyOnlyForRoutesWithoutSideEffects(t *testing.T) {
	withPass := schema.GroupKind{Group: "test", Kind: "WithPass"}
	noPass := schema.GroupKind{Group: "test", Kind: "NoPass"}
	h := &httpRouteConfigurationTranslator{pluginPass: TranslationPassPlugins{withPass: &TranslationPass{}}}
	policies := func(gk schema.GroupKind) ir.AttachedPolicies {
		return ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{gk: {{}}}}
	}
	service := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")
	plain := func() ir.HttpRouteRuleMatchIR {
		return ir.HttpRouteRuleMatchIR{
			Parent:           &ir.HttpRouteIR{AttachedPolicies: policies(noPass)},
			AttachedPolicies: policies(noPass),
			Backends:         []ir.HttpBackend{{Backend: ir.BackendRefIR{ClusterName: "c", BackendObject: &service}}},
		}
	}

	in := plain()
	_, ok := h.routeCacheKey(&in, "name")
	require.True(t, ok, "a route whose policies have no pass should be cacheable")

	for name, change := range map[string]func(*ir.HttpRouteRuleMatchIR){
		"no parent":         func(in *ir.HttpRouteRuleMatchIR) { in.Parent = nil },
		"delegates":         func(in *ir.HttpRouteRuleMatchIR) { in.Delegates = true },
		"delegated":         func(in *ir.HttpRouteRuleMatchIR) { in.DelegatingParent = &ir.HttpRouteRuleMatchIR{} },
		"acceptance error":  func(in *ir.HttpRouteRuleMatchIR) { in.RouteAcceptanceError = errors.New("bad") },
		"replacement error": func(in *ir.HttpRouteRuleMatchIR) { in.RouteReplacementError = errors.New("bad") },
		"no backends":       func(in *ir.HttpRouteRuleMatchIR) { in.Backends = nil },
		"extension ref":     func(in *ir.HttpRouteRuleMatchIR) { in.ExtensionRefs = policies(withPass) },
		"rule policy":       func(in *ir.HttpRouteRuleMatchIR) { in.AttachedPolicies = policies(withPass) },
		"route policy":      func(in *ir.HttpRouteRuleMatchIR) { in.Parent.AttachedPolicies = policies(withPass) },
		"backend policy":    func(in *ir.HttpRouteRuleMatchIR) { in.Backends[0].AttachedPolicies = policies(withPass) },
		"backend kind with pass": func(in *ir.HttpRouteRuleMatchIR) {
			obj := ir.NewBackendObjectIR(ir.ObjectSource{Group: withPass.Group, Kind: withPass.Kind, Namespace: "ns", Name: "b"}, 0, "", "")
			in.Backends[0].Backend.BackendObject = &obj
		},
		"backend TLS policy": func(in *ir.HttpRouteRuleMatchIR) {
			obj := service
			obj.AttachedPolicies = policies(wellknown.BackendTLSPolicyGVK.GroupKind())
			in.Backends[0].Backend.BackendObject = &obj
		},
	} {
		t.Run(name, func(t *testing.T) {
			in := plain()
			change(&in)
			_, ok := h.routeCacheKey(&in, "name")
			require.False(t, ok, "a route with %s should not be cacheable", name)
		})
	}
}

func TestShareCachedRoutesOnlyWithoutRouteEditingPlugins(t *testing.T) {
	withPass := schema.GroupKind{Group: "test", Kind: "WithPass"}
	policies := ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{withPass: {{}}}}
	h := &httpRouteConfigurationTranslator{pluginPass: TranslationPassPlugins{withPass: &TranslationPass{}}}

	require.True(t, h.shareCachedRoutes(&ir.VirtualHost{}), "routes should be shared without route config or vhost policies")
	require.False(t, h.shareCachedRoutes(&ir.VirtualHost{AttachedPolicies: policies}), "a vhost policy may edit routes")
	h.attachedPolicies = policies
	require.False(t, h.shareCachedRoutes(&ir.VirtualHost{}), "a listener policy may edit routes")
	h.attachedPolicies = ir.AttachedPolicies{}
	h.gw.AttachedHttpPolicies = policies
	require.False(t, h.shareCachedRoutes(&ir.VirtualHost{}), "a Gateway policy may edit routes")
}
