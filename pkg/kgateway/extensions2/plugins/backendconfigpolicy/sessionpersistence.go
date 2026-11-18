package backendconfigpolicy

import (
	"errors"
	"fmt"
	"slices"
	"strings"

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
	// absoluteTimeoutIgnored is set when the absoluteTimeout has no effect (Session cookie or header).
	absoluteTimeoutIgnored bool
}

func (s *sessionPersistenceIR) Equals(other *sessionPersistenceIR) bool {
	if s == nil || other == nil {
		return s == other
	}
	return proto.Equal(s.perRoute, other.perRoute) && proto.Equal(s.cookieState, other.cookieState) &&
		s.absoluteTimeoutIgnored == other.absoluteTimeoutIgnored
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
		// Header sessions never use the timeout.
		return true
	}
	lifetime := kgateway.SessionCookieLifetimeType
	if in.Cookie != nil {
		lifetime = ptr.Deref(in.Cookie.LifetimeType, kgateway.SessionCookieLifetimeType)
	}
	return lifetime == kgateway.SessionCookieLifetimeType
}

const (
	msgAbsoluteTimeoutIgnored = "spec.sessionPersistence.absoluteTimeout only has an effect for Cookie sessions " +
		"with spec.sessionPersistence.cookie.lifetimeType Permanent"
	msgNoEffectOnBackendFmt = "spec.sessionPersistence has no effect on %s %s/%s: %s"
)

// PartiallyValidMessages returns the message for the PartiallyValid reason of the Accepted condition, keyed by
// policy ref ID, for the BackendConfigPolicies attached to backend. It is judged on the merged config that is
// actually applied to the backend, and reported on the policy whose sessionPersistence is applied: an ignored
// absoluteTimeout and backends where session persistence has no effect. Load balancer hash policies are not
// reported: they still pick the host for requests without a session. It returns nil when there is nothing to report.
func PartiallyValidMessages(policies []ir.PolicyAtt, backend ir.BackendObjectIR) map[string]string {
	if !slices.ContainsFunc(policies, hasSessionPersistence) {
		return nil
	}
	merged := mergePolicies(policies)
	// Like translation, nothing is applied when a policy has errors.
	if len(merged.Errors) > 0 {
		return nil
	}
	pol, ok := merged.PolicyIr.(*BackendConfigPolicyIR)
	if !ok || pol.sessionPersistence == nil {
		return nil
	}
	owners := merged.MergeOrigins.Get("sessionPersistence")
	if len(owners) != 1 {
		return nil
	}

	var msgs []string
	if pol.sessionPersistence.absoluteTimeoutIgnored {
		msgs = append(msgs, msgAbsoluteTimeoutIgnored)
	}
	if note, report := sessionPersistenceBackendNote(backend); report {
		gk := backend.GetGroupKind()
		msgs = append(msgs, fmt.Sprintf(msgNoEffectOnBackendFmt, gk.Kind, backend.GetNamespace(), backend.GetName(), note))
	}
	if len(msgs) == 0 {
		return nil
	}
	slices.Sort(msgs)
	return map[string]string{owners[0]: strings.Join(msgs, "; ")}
}

// sessionPersistenceBackendNote returns a note about how session persistence behaves for backend, and whether
// the note is reported in the policy status (report) or only logged. It returns "" when there is nothing to note.
// Backends with a single host (e.g. AWS Lambda or a static IP) are not noted: every request already goes to the
// same host, so the session is trivially kept.
func sessionPersistenceBackendNote(backend ir.BackendObjectIR) (note string, report bool) {
	be, ok := backend.Obj.(*kgateway.Backend)
	if !ok {
		return "", false
	}
	switch {
	case be.Spec.Gcp != nil:
		// Users of GCP backends likely know this, so it is only logged.
		return "GCP backends are reached through Google front ends, so a session is pinned to a front end, not an instance", false
	case be.Spec.DynamicForwardProxy != nil:
		// Requests are load balanced by a per host sub-cluster that does not use the session's host, so
		// session persistence has no effect (requests are never sent to a previous host either).
		return "dynamic forward proxy backends load balance each request within the requested host's addresses " +
			"and do not use the session", true
	}
	return "", false
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
