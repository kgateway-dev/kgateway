package backendconfigpolicy

import (
	"errors"
	"net"
	"slices"

	stateful_cookie "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/cookie/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils/sessionpersistence"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

// sessionPersistenceIR is the translated session persistence config of a BackendConfigPolicy.
// Exactly one of perRoute and cookieState is set.
type sessionPersistenceIR struct {
	// perRoute is the per route config of the stateful session filter (a StatefulSessionPerRoute),
	// marshaled once here so route translation can reuse it without marshaling it again per route.
	perRoute *anypb.Any
	// cookieState is set instead of perRoute when the cookie name is not specified, so a
	// unique name can be generated for each backend when the route is translated.
	cookieState *stateful_cookie.CookieBasedSessionState
}

func (s *sessionPersistenceIR) Equals(other *sessionPersistenceIR) bool {
	if s == nil || other == nil {
		return s == other
	}
	return proto.Equal(s.perRoute, other.perRoute) && proto.Equal(s.cookieState, other.cookieState)
}

// perRouteFor returns the per route config of the stateful session filter for backend,
// marshaled as an Any. The returned value is shared and must not be modified.
func (s *sessionPersistenceIR) perRouteFor(backend *ir.BackendObjectIR) (*anypb.Any, error) {
	if s.perRoute != nil {
		return s.perRoute, nil
	}
	if backend == nil {
		return nil, errors.New("session persistence cookie name cannot be generated without a backend")
	}
	state := proto.Clone(s.cookieState).(*stateful_cookie.CookieBasedSessionState)
	state.Cookie.Name = sessionpersistence.GeneratedCookieName(backend.ResourceName())
	perRoute, err := sessionpersistence.PerRoute(state)
	if err != nil {
		return nil, err
	}
	return utils.MessageToAny(perRoute)
}

func translateSessionPersistence(in *kgateway.SessionPersistence) (*sessionPersistenceIR, error) {
	if in == nil {
		return nil, nil
	}
	cfg, err := sessionpersistence.ConfigFromAPI(in)
	if err != nil {
		return nil, err
	}
	if cfg.Type == sessionpersistence.Cookie && cfg.Name == "" {
		// The cookie name is generated for each backend when the route is translated.
		return &sessionPersistenceIR{cookieState: sessionpersistence.CookieState(cfg)}, nil
	}
	perRoute, err := sessionpersistence.Build(cfg)
	if err != nil {
		return nil, err
	}
	perRouteAny, err := utils.MessageToAny(perRoute)
	if err != nil {
		return nil, err
	}
	return &sessionPersistenceIR{perRoute: perRouteAny}, nil
}

// absoluteTimeoutIgnored reports whether in sets an absoluteTimeout that has no effect.
// Envoy can only enforce the session lifetime through the cookie Max-Age, which a Session
// cookie (the default lifetime) does not have, so the timeout only applies to Permanent cookies.
func absoluteTimeoutIgnored(in *kgateway.SessionPersistence) bool {
	if in == nil || in.AbsoluteTimeout == nil {
		return false
	}
	if ptr.Deref(in.Type, kgateway.CookieBasedSessionPersistence) != kgateway.CookieBasedSessionPersistence {
		return false
	}
	lifetime := kgateway.SessionCookieLifetimeType
	if in.Cookie != nil {
		lifetime = ptr.Deref(in.Cookie.LifetimeType, kgateway.SessionCookieLifetimeType)
	}
	return lifetime == kgateway.SessionCookieLifetimeType
}

// hasHashPolicies reports whether the load balancer config uses hash policies.
func hasHashPolicies(lb *kgateway.LoadBalancer) bool {
	if lb == nil {
		return false
	}
	return (lb.RingHash != nil && len(lb.RingHash.HashPolicies) > 0) ||
		(lb.Maglev != nil && len(lb.Maglev.HashPolicies) > 0)
}

// sessionPersistenceHasNoEffect returns a reason when session persistence has no
// effect for backend, because the backend has at most one host to pin to.
func sessionPersistenceHasNoEffect(backend ir.BackendObjectIR) string {
	be, ok := backend.Obj.(*kgateway.Backend)
	if !ok {
		return ""
	}
	switch {
	case be.Spec.Aws != nil:
		return "AWS backends have no endpoints to pin to"
	case be.Spec.Gcp != nil:
		return "GCP backends have a single host"
	case be.Spec.DynamicForwardProxy != nil:
		return "dynamic forward proxy backends resolve the host per request"
	case be.Spec.Static != nil && len(be.Spec.Static.Hosts) == 1 && net.ParseIP(be.Spec.Static.Hosts[0].Host) != nil:
		// A single hostname is resolved by DNS and may have several addresses to pin to.
		return "static backends with a single IP address have nothing to pin to"
	}
	return ""
}

type backendConfigPolicyGwPass struct {
	ir.UnimplementedProxyTranslationPass

	// needStatefulSession tracks the filter chains that need the stateful_session filter.
	needStatefulSession map[string]bool
}

var (
	_ ir.ProxyTranslationPass       = &backendConfigPolicyGwPass{}
	_ ir.BackendObjectPolicyApplier = &backendConfigPolicyGwPass{}
)

func newGatewayTranslationPass(_ ir.GwTranslationCtx, _ reporter.Reporter) ir.ProxyTranslationPass {
	return &backendConfigPolicyGwPass{
		needStatefulSession: make(map[string]bool),
	}
}

// ApplyForBackendObjectPolicies sets the per route session persistence config for a backend
// that has a BackendConfigPolicy with session persistence attached. It runs for every route
// backend on every translation, so it only merges the policies when one of them sets
// session persistence.
func (p *backendConfigPolicyGwPass) ApplyForBackendObjectPolicies(policies []ir.PolicyAtt, pCtx *ir.RouteBackendContext) error {
	if !slices.ContainsFunc(policies, hasSessionPersistence) {
		return nil
	}
	// Usually a single policy is attached; merging it would only copy it, so use it directly.
	att := policies[0]
	if len(policies) > 1 {
		att = mergePolicies(policies)
	}
	// Like cluster translation, skip the merged policy if any of the policies has errors.
	if len(att.Errors) > 0 {
		return nil
	}
	pol, ok := att.PolicyIr.(*BackendConfigPolicyIR)
	if !ok || pol.sessionPersistence == nil {
		return nil
	}
	perRoute, err := pol.sessionPersistence.perRouteFor(pCtx.Backend)
	if err != nil {
		return err
	}
	pCtx.TypedFilterConfig.AddTypedConfig(sessionpersistence.FilterName, perRoute)
	p.needStatefulSession[pCtx.FilterChainName] = true
	return nil
}

func hasSessionPersistence(att ir.PolicyAtt) bool {
	pol, ok := att.PolicyIr.(*BackendConfigPolicyIR)
	return ok && pol.sessionPersistence != nil
}

// HttpFilters adds the stateful_session filter, disabled by default, to filter chains that
// have routes to a backend with session persistence. It is enabled per route.
func (p *backendConfigPolicyGwPass) HttpFilters(_ ir.HttpFiltersContext, fcc ir.FilterChainCommon) ([]filters.StagedHttpFilter, error) {
	if !p.needStatefulSession[fcc.FilterChainName] {
		return nil, nil
	}
	return []filters.StagedHttpFilter{sessionpersistence.DisabledFilter()}, nil
}
