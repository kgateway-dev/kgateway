package irtranslator

import (
	"sync"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// routeCache keeps the Envoy routes translated for each route configuration, so a Gateway
// rebuild only translates the routes that changed. Each Gateway translation keeps only the
// entries it used, so routes, route configurations and Gateways that went away are dropped.
type routeCache struct {
	mu sync.Mutex
	// gateways holds each Gateway's last translated routes, by route configuration name.
	gateways map[types.NamespacedName]map[string]map[routeKey]*envoyroutev3.Route
	// translating holds what the Gateway translations in progress have used so far.
	translating map[types.NamespacedName]map[string]map[routeKey]*envoyroutev3.Route
}

// routeKey identifies a route's translation input. The route IR is immutable once built, so
// the pointer stands for its contents; the rule and match indexes pick the match within it.
// The rest of a cacheable route's input is the name it is translated under.
type routeKey struct {
	route         *ir.HttpRouteIR
	ruleIndex     int
	matchIndex    int
	name          string
	generatedName string
}

// routeCacheGeneration is the cache of one route configuration for one translation: it reads
// the previous translation's entries and records the ones this translation uses.
type routeCacheGeneration struct {
	gateway         types.NamespacedName
	routeConfigName string
	prev, next      map[routeKey]*envoyroutev3.Route
}

// beginGateway starts a translation of gateway. Route configurations translated until
// endGateway replace the ones cached for it.
func (c *routeCache) beginGateway(gateway types.NamespacedName) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.translating == nil {
		c.translating = map[types.NamespacedName]map[string]map[routeKey]*envoyroutev3.Route{}
	}
	c.translating[gateway] = map[string]map[routeKey]*envoyroutev3.Route{}
}

func (c *routeCache) endGateway(gateway types.NamespacedName) {
	c.mu.Lock()
	defer c.mu.Unlock()
	used, ok := c.translating[gateway]
	if !ok {
		return
	}
	delete(c.translating, gateway)
	if c.gateways == nil {
		c.gateways = map[types.NamespacedName]map[string]map[routeKey]*envoyroutev3.Route{}
	}
	c.gateways[gateway] = used
}

// forgetGateway drops what is cached for a Gateway that was deleted.
func (c *routeCache) forgetGateway(gateway types.NamespacedName) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.gateways, gateway)
}

// begin starts translating one route configuration of a Gateway being translated. It returns
// nil, disabling the cache, outside a Gateway translation.
func (c *routeCache) begin(gateway types.NamespacedName, routeConfigName string) *routeCacheGeneration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.translating[gateway]; !ok {
		return nil
	}
	prev := c.gateways[gateway][routeConfigName]
	return &routeCacheGeneration{
		gateway:         gateway,
		routeConfigName: routeConfigName,
		prev:            prev,
		next:            make(map[routeKey]*envoyroutev3.Route, len(prev)),
	}
}

func (c *routeCache) end(gen *routeCacheGeneration) {
	if gen == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if used, ok := c.translating[gen.gateway]; ok {
		used[gen.routeConfigName] = gen.next
	}
}

// get returns the cached route. Unless shared, it returns a copy, since route config and
// virtual host plugins may edit routes in place.
func (g *routeCacheGeneration) get(key routeKey, shared bool) *envoyroutev3.Route {
	if g == nil {
		return nil
	}
	cached, ok := g.prev[key]
	if !ok {
		return nil
	}
	g.next[key] = cached
	if shared {
		return cached
	}
	return proto.Clone(cached).(*envoyroutev3.Route)
}

// put caches route and returns the route the caller should use: route itself if shared,
// otherwise a copy, so later edits do not reach the cache.
func (g *routeCacheGeneration) put(key routeKey, route *envoyroutev3.Route, shared bool) *envoyroutev3.Route {
	if g == nil {
		return route
	}
	g.next[key] = route
	if shared {
		return route
	}
	return proto.Clone(route).(*envoyroutev3.Route)
}

// shareCachedRoutes reports whether routes in virtualHost can be shared with the cache. Only
// route config and virtual host plugins edit routes after they are translated, so they can be
// shared when no such policy has a translation pass. Route replacement copies the route first.
func (h *httpRouteConfigurationTranslator) shareCachedRoutes(virtualHost *ir.VirtualHost) bool {
	return !h.hasPass(h.attachedPolicies) && !h.hasPass(h.gw.AttachedHttpPolicies) && !h.hasPass(virtualHost.AttachedPolicies)
}

// routeCacheKey returns the cache key for a route whose translation depends only on the route
// itself: one that does not delegate, carries no errors, and whose policies and backends have
// no translation pass to run. Plugins and status reporting never see such a route, so replaying
// its translation skips no side effects.
func (h *httpRouteConfigurationTranslator) routeCacheKey(in *ir.HttpRouteRuleMatchIR, generatedName string) (routeKey, bool) {
	if in.Parent == nil || in.Delegates || in.DelegatingParent != nil ||
		in.RouteAcceptanceError != nil || in.RouteReplacementError != nil || len(in.Backends) == 0 {
		return routeKey{}, false
	}
	if h.hasPass(in.ExtensionRefs) || h.hasPass(in.AttachedPolicies) || h.hasPass(in.Parent.AttachedPolicies) {
		return routeKey{}, false
	}
	for _, backend := range in.Backends {
		if h.hasPass(backend.AttachedPolicies) {
			return routeKey{}, false
		}
		if obj := backend.Backend.BackendObject; obj != nil {
			if h.pluginPass[obj.GetGroupKind()] != nil ||
				len(obj.AttachedPolicies.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()]) > 0 {
				return routeKey{}, false
			}
		}
	}
	return routeKey{
		route:         in.Parent,
		ruleIndex:     in.RuleIndex,
		matchIndex:    in.MatchIndex,
		name:          in.Name,
		generatedName: generatedName,
	}, true
}

func (h *httpRouteConfigurationTranslator) hasPass(policies ir.AttachedPolicies) bool {
	for gk := range policies.Policies {
		if h.pluginPass[gk] != nil {
			return true
		}
	}
	return false
}
