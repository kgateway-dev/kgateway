package backendconfigpolicy

import (
	"errors"
	"testing"
	"time"

	stateful_sessionv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/stateful_session/v3"
	stateful_cookie "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/cookie/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils/sessionpersistence"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

// perRouteOf unmarshals the per route session persistence config stored as an Any.
func perRouteOf(t *testing.T, m proto.Message) *stateful_sessionv3.StatefulSessionPerRoute {
	t.Helper()
	a, ok := m.(*anypb.Any)
	require.True(t, ok, "per route config should be a pre-marshaled Any, got %T", m)
	perRoute := &stateful_sessionv3.StatefulSessionPerRoute{}
	require.NoError(t, a.UnmarshalTo(perRoute))
	return perRoute
}

func cookieStateOf(t *testing.T, m proto.Message) *stateful_cookie.CookieBasedSessionState {
	t.Helper()
	state := &stateful_cookie.CookieBasedSessionState{}
	require.NoError(t, perRouteOf(t, m).GetStatefulSession().GetSessionState().GetTypedConfig().UnmarshalTo(state))
	return state
}

func TestTranslateSessionPersistence(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		got, err := translateSessionPersistence(nil)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("cookie with name", func(t *testing.T) {
		got, err := translateSessionPersistence(&kgateway.SessionPersistence{
			AbsoluteTimeout: new(gwv1.Duration("10s")),
			Cookie: &kgateway.SessionPersistenceCookieConfig{
				Name:         new(kgateway.CookieName("my-session")),
				Path:         new("/app"),
				LifetimeType: new(kgateway.PermanentCookieLifetimeType),
			},
		})
		require.NoError(t, err)
		require.NotNil(t, got.perRoute, "a named cookie is fully built in the IR")
		assert.Nil(t, got.cookieState)
		state := cookieStateOf(t, got.perRoute)
		assert.Equal(t, "my-session", state.GetCookie().GetName())
		assert.Equal(t, "/app", state.GetCookie().GetPath())
		assert.Equal(t, 10*time.Second, state.GetCookie().GetTtl().AsDuration())
	})

	t.Run("cookie without name", func(t *testing.T) {
		got, err := translateSessionPersistence(&kgateway.SessionPersistence{
			AbsoluteTimeout: new(gwv1.Duration("10s")),
			Cookie:          &kgateway.SessionPersistenceCookieConfig{},
		})
		require.NoError(t, err)
		assert.Nil(t, got.perRoute, "an unnamed cookie is built per backend")
		require.NotNil(t, got.cookieState)
		assert.Equal(t, "/", got.cookieState.GetCookie().GetPath())
		assert.Nil(t, got.cookieState.GetCookie().GetTtl(), "lifetime defaults to Session, which has no ttl")
	})

	t.Run("header", func(t *testing.T) {
		got, err := translateSessionPersistence(&kgateway.SessionPersistence{
			Type:   new(kgateway.HeaderBasedSessionPersistence),
			Header: &kgateway.SessionPersistenceHeaderConfig{Name: "x-session"},
		})
		require.NoError(t, err)
		require.NotNil(t, got.perRoute)
		assert.Equal(t, "Header/x-session", sessionpersistence.SessionKey(perRouteOf(t, got.perRoute)))
	})

	t.Run("header without header config", func(t *testing.T) {
		_, err := translateSessionPersistence(&kgateway.SessionPersistence{
			Type: new(kgateway.HeaderBasedSessionPersistence),
		})
		assert.EqualError(t, err, "sessionPersistence.header must be specified for Header type")
	})

	t.Run("invalid absolute timeout", func(t *testing.T) {
		_, err := translateSessionPersistence(&kgateway.SessionPersistence{
			AbsoluteTimeout: new(gwv1.Duration("bogus")),
			Cookie:          &kgateway.SessionPersistenceCookieConfig{},
		})
		assert.ErrorContains(t, err, "invalid sessionPersistence.absoluteTimeout")
	})
}

func TestSessionPersistenceIREquals(t *testing.T) {
	named := func(name string) *sessionPersistenceIR {
		sp, err := translateSessionPersistence(&kgateway.SessionPersistence{
			Cookie: &kgateway.SessionPersistenceCookieConfig{Name: new(kgateway.CookieName(name))},
		})
		require.NoError(t, err)
		return sp
	}
	unnamed := func() *sessionPersistenceIR {
		sp, err := translateSessionPersistence(&kgateway.SessionPersistence{Cookie: &kgateway.SessionPersistenceCookieConfig{}})
		require.NoError(t, err)
		return sp
	}

	assert.True(t, (*sessionPersistenceIR)(nil).Equals(nil), "nil equals nil")
	assert.False(t, named("a").Equals(nil), "set does not equal nil")
	assert.True(t, named("a").Equals(named("a")), "same config is equal")
	assert.False(t, named("a").Equals(named("b")), "different cookie name is not equal")
	assert.True(t, unnamed().Equals(unnamed()), "same unnamed config is equal")
	assert.False(t, named("a").Equals(unnamed()), "named and unnamed are not equal")

	a := &BackendConfigPolicyIR{sessionPersistence: named("a")}
	b := &BackendConfigPolicyIR{sessionPersistence: named("a")}
	assert.True(t, a.Equals(b), "identical session persistence should be equal")
	b.sessionPersistence = named("b")
	assert.False(t, a.Equals(b), "changing session persistence must change the policy IR")
	b.sessionPersistence = nil
	assert.False(t, a.Equals(b), "removing session persistence must change the policy IR")
}

func TestMergeSessionPersistence(t *testing.T) {
	sp := func(name string) *sessionPersistenceIR {
		out, err := translateSessionPersistence(&kgateway.SessionPersistence{
			Cookie: &kgateway.SessionPersistenceCookieConfig{Name: new(kgateway.CookieName(name))},
		})
		require.NoError(t, err)
		return out
	}
	gk := schema.GroupKind{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"}
	merge := func(pols ...*BackendConfigPolicyIR) *BackendConfigPolicyIR {
		var atts []ir.PolicyAtt
		for i, p := range pols {
			atts = append(atts, ir.PolicyAtt{GroupKind: gk, PolicyRef: &ir.AttachedPolicyRef{Name: string(rune('a' + i))}, PolicyIr: p})
		}
		return policy.MergePolicies(atts, mergeBackendConfigPolicies, "").PolicyIr.(*BackendConfigPolicyIR)
	}

	older := &BackendConfigPolicyIR{ct: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), sessionPersistence: sp("older")}
	newer := &BackendConfigPolicyIR{ct: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), sessionPersistence: sp("newer")}
	assert.True(t, merge(older, newer).sessionPersistence.Equals(sp("older")), "older sessionPersistence should win")

	unset := &BackendConfigPolicyIR{ct: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
	assert.True(t, merge(unset, newer).sessionPersistence.Equals(sp("newer")), "newer sessionPersistence should fill in")
}

func TestApplyForBackendObjectPolicies(t *testing.T) {
	gk := schema.GroupKind{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"}
	backend := func(name string) *ir.BackendObjectIR {
		b := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "default", Name: name}, 3000, "", "")
		return &b
	}
	att := func(name string, created time.Time, pol *BackendConfigPolicyIR, errs ...error) ir.PolicyAtt {
		pol.ct = created
		return ir.PolicyAtt{GroupKind: gk, PolicyRef: &ir.AttachedPolicyRef{Name: name}, PolicyIr: pol, Errors: errs}
	}
	namedSP := func(name string) *sessionPersistenceIR {
		sp, err := translateSessionPersistence(&kgateway.SessionPersistence{
			Cookie: &kgateway.SessionPersistenceCookieConfig{Name: new(kgateway.CookieName(name))},
		})
		require.NoError(t, err)
		return sp
	}
	unnamed, err := translateSessionPersistence(&kgateway.SessionPersistence{Cookie: &kgateway.SessionPersistenceCookieConfig{}})
	require.NoError(t, err)
	placeholderName := unnamed.cookieState.GetCookie().GetName()
	older, newer := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	cookieName := func(pCtx *ir.RouteBackendContext) string {
		perRoute := pCtx.TypedFilterConfig.GetTypedConfig(sessionpersistence.FilterName)
		require.NotNil(t, perRoute, "per route session persistence config should be set")
		return cookieStateOf(t, perRoute).GetCookie().GetName()
	}

	pass := newGatewayTranslationPass(ir.GwTranslationCtx{}, nil).(*backendConfigPolicyGwPass)

	ctxA := &ir.RouteBackendContext{FilterChainName: "fc", Backend: backend("a")}
	require.NoError(t, pass.ApplyForBackendObjectPolicies([]ir.PolicyAtt{att("p", older, &BackendConfigPolicyIR{sessionPersistence: unnamed})}, ctxA))
	assert.Equal(t, sessionpersistence.GeneratedCookieName(backend("a").ResourceName()), cookieName(ctxA))
	assert.Equal(t, placeholderName, unnamed.cookieState.GetCookie().GetName(), "generating a name must not mutate the IR")

	ctxB := &ir.RouteBackendContext{FilterChainName: "fc", Backend: backend("b")}
	require.NoError(t, pass.ApplyForBackendObjectPolicies([]ir.PolicyAtt{att("p", older, &BackendConfigPolicyIR{sessionPersistence: unnamed})}, ctxB))
	assert.NotEqual(t, cookieName(ctxA), cookieName(ctxB), "each backend gets its own cookie name")

	merged := &ir.RouteBackendContext{FilterChainName: "fc", Backend: backend("c")}
	require.NoError(t, pass.ApplyForBackendObjectPolicies([]ir.PolicyAtt{
		att("newer", newer, &BackendConfigPolicyIR{sessionPersistence: namedSP("newer")}),
		att("older", older, &BackendConfigPolicyIR{sessionPersistence: namedSP("older")}),
	}, merged))
	assert.Equal(t, "older", cookieName(merged), "policies should be merged with the older policy winning")

	noSP := &ir.RouteBackendContext{FilterChainName: "other", Backend: backend("d")}
	require.NoError(t, pass.ApplyForBackendObjectPolicies([]ir.PolicyAtt{att("p", older, &BackendConfigPolicyIR{})}, noSP))
	assert.Nil(t, noSP.TypedFilterConfig.GetTypedConfig(sessionpersistence.FilterName), "no config without session persistence")

	withErrors := &ir.RouteBackendContext{FilterChainName: "other", Backend: backend("e")}
	require.NoError(t, pass.ApplyForBackendObjectPolicies([]ir.PolicyAtt{
		att("bad", older, &BackendConfigPolicyIR{sessionPersistence: namedSP("bad")}, errors.New("invalid policy")),
	}, withErrors))
	assert.Nil(t, withErrors.TypedFilterConfig.GetTypedConfig(sessionpersistence.FilterName), "policies with errors are skipped, as in cluster translation")

	fs, err := pass.HttpFilters(ir.HttpFiltersContext{}, ir.FilterChainCommon{FilterChainName: "fc"})
	require.NoError(t, err)
	require.Len(t, fs, 1, "filter chain with session persistence routes gets the filter")
	assert.Equal(t, sessionpersistence.FilterName, fs[0].Filter.GetName())
	assert.True(t, fs[0].Filter.GetDisabled(), "filter is disabled by default and enabled per route")

	fs, err = pass.HttpFilters(ir.HttpFiltersContext{}, ir.FilterChainCommon{FilterChainName: "other"})
	require.NoError(t, err)
	assert.Empty(t, fs, "filter chain without session persistence routes gets no filter")
}

func TestSessionPersistenceBackendNote(t *testing.T) {
	be := func(spec kgateway.BackendSpec) ir.BackendObjectIR {
		return ir.BackendObjectIR{Obj: &kgateway.Backend{ObjectMeta: metav1.ObjectMeta{Name: "be"}, Spec: spec}}
	}
	host := kgateway.Host{Host: "example.com", Port: 80}
	ipHost := kgateway.Host{Host: "10.0.0.1", Port: 80}
	note := func(spec kgateway.BackendSpec) (string, bool) { return sessionPersistenceBackendNote(be(spec)) }
	n, report := note(kgateway.BackendSpec{Gcp: &kgateway.GcpBackend{}})
	assert.NotEmpty(t, n, "GCP is noted")
	assert.False(t, report, "GCP is only logged, not reported in status")
	n, report = note(kgateway.BackendSpec{DynamicForwardProxy: &kgateway.DynamicForwardProxyBackend{}})
	assert.NotEmpty(t, n, "DFP is noted")
	assert.True(t, report, "DFP is reported in status")
	for name, spec := range map[string]kgateway.BackendSpec{
		"AWS Lambda has a single host, so the session is trivially kept": {Aws: &kgateway.AwsBackend{Lambda: &kgateway.AwsLambda{}}},
		"AWS EC2 has instances to pin to":                                {Aws: &kgateway.AwsBackend{Ec2: &kgateway.AwsEc2{}}},
		"single IP static has a single host":                             {Static: &kgateway.StaticBackend{Hosts: []kgateway.Host{ipHost}}},
		"single hostname static may resolve to several addresses":        {Static: &kgateway.StaticBackend{Hosts: []kgateway.Host{host}}},
		"multi host static":                                              {Static: &kgateway.StaticBackend{Hosts: []kgateway.Host{host, host}}},
	} {
		n, report := note(spec)
		assert.Empty(t, n, name)
		assert.False(t, report, name)
	}
	n, _ = sessionPersistenceBackendNote(ir.BackendObjectIR{})
	assert.Empty(t, n, "non Backend kinds such as Service")
}

func TestAbsoluteTimeoutIgnored(t *testing.T) {
	timeout := new(gwv1.Duration("1h"))
	tests := []struct {
		name string
		in   *kgateway.SessionPersistence
		want bool
	}{
		{name: "nil", in: nil},
		{name: "no timeout", in: &kgateway.SessionPersistence{Cookie: &kgateway.SessionPersistenceCookieConfig{}}},
		{name: "default lifetime", in: &kgateway.SessionPersistence{AbsoluteTimeout: timeout, Cookie: &kgateway.SessionPersistenceCookieConfig{}}, want: true},
		{name: "session lifetime", in: &kgateway.SessionPersistence{AbsoluteTimeout: timeout, Cookie: &kgateway.SessionPersistenceCookieConfig{
			LifetimeType: new(kgateway.SessionCookieLifetimeType),
		}}, want: true},
		{name: "permanent lifetime", in: &kgateway.SessionPersistence{AbsoluteTimeout: timeout, Cookie: &kgateway.SessionPersistenceCookieConfig{
			LifetimeType: new(kgateway.PermanentCookieLifetimeType),
		}}},
		{name: "header sessions never use the timeout", in: &kgateway.SessionPersistence{
			AbsoluteTimeout: timeout,
			Type:            new(kgateway.HeaderBasedSessionPersistence),
			Header:          &kgateway.SessionPersistenceHeaderConfig{Name: "x-session"},
		}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, absoluteTimeoutIgnored(tt.in))
		})
	}
}

func TestPartiallyValidMessages(t *testing.T) {
	gk := schema.GroupKind{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"}
	older, newer := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	att := func(name string, pol *BackendConfigPolicyIR, errs ...error) ir.PolicyAtt {
		return ir.PolicyAtt{GroupKind: gk, PolicyRef: &ir.AttachedPolicyRef{Group: gk.Group, Kind: gk.Kind, Namespace: "default", Name: name}, PolicyIr: pol, Errors: errs}
	}
	id := func(name string) string {
		return (&ir.AttachedPolicyRef{Group: gk.Group, Kind: gk.Kind, Namespace: "default", Name: name}).ID()
	}
	sp := func(timeoutIgnored bool) *sessionPersistenceIR {
		return &sessionPersistenceIR{perRoute: &anypb.Any{}, absoluteTimeoutIgnored: timeoutIgnored}
	}
	service := ir.BackendObjectIR{}
	dfp := ir.NewBackendObjectIR(ir.ObjectSource{Group: "gateway.kgateway.dev", Kind: "Backend", Namespace: "default", Name: "dfp"}, 0, "", "")
	dfp.Obj = &kgateway.Backend{Spec: kgateway.BackendSpec{DynamicForwardProxy: &kgateway.DynamicForwardProxyBackend{}}}

	tests := []struct {
		name     string
		policies []ir.PolicyAtt
		backend  ir.BackendObjectIR
		want     map[string]string
	}{
		{name: "no session persistence", policies: []ir.PolicyAtt{att("a", &BackendConfigPolicyIR{ct: older, loadBalancerConfig: &LoadBalancerConfigIR{}})}, backend: service},
		{name: "nothing to report", policies: []ir.PolicyAtt{att("a", &BackendConfigPolicyIR{ct: older, sessionPersistence: sp(false)})}, backend: service},
		{
			name:     "ignored absoluteTimeout",
			policies: []ir.PolicyAtt{att("a", &BackendConfigPolicyIR{ct: older, sessionPersistence: sp(true)})},
			backend:  service,
			want:     map[string]string{id("a"): msgAbsoluteTimeoutIgnored},
		},
		{
			name: "only the applied session persistence is reported",
			policies: []ir.PolicyAtt{
				att("a", &BackendConfigPolicyIR{ct: older, sessionPersistence: sp(false)}),
				att("b", &BackendConfigPolicyIR{ct: newer, sessionPersistence: sp(true)}),
			},
			backend: service,
		},
		{
			name:     "backend reported in status",
			policies: []ir.PolicyAtt{att("a", &BackendConfigPolicyIR{ct: older, sessionPersistence: sp(false)})},
			backend:  dfp,
			want: map[string]string{id("a"): "spec.sessionPersistence has no effect on Backend default/dfp: dynamic forward proxy backends " +
				"load balance each request within the requested host's addresses and do not use the session"},
		},
		{
			name:     "nothing is applied when a policy has errors",
			policies: []ir.PolicyAtt{att("a", &BackendConfigPolicyIR{ct: older, sessionPersistence: sp(true)}, errors.New("bad"))},
			backend:  service,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, PartiallyValidMessages(tt.policies, tt.backend))
		})
	}

	a := &sessionPersistenceIR{perRoute: &anypb.Any{}, absoluteTimeoutIgnored: true}
	assert.False(t, a.Equals(sp(false)), "absoluteTimeoutIgnored must be part of Equals")
}
