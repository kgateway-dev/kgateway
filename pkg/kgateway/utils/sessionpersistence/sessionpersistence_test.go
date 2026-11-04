package sessionpersistence

import (
	"testing"
	"time"

	stateful_sessionv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/stateful_session/v3"
	stateful_cookie "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/cookie/v3"
	stateful_header "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/header/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
)

func TestCookieState(t *testing.T) {
	timeout := 10 * time.Second
	tests := []struct {
		name     string
		cfg      Config
		wantName string
		wantPath string
		wantTTL  *time.Duration
	}{
		{
			name:     "defaults path to root and keeps absolute timeout as ttl",
			cfg:      Config{Name: "my-session", AbsoluteTimeout: &timeout},
			wantName: "my-session",
			wantPath: "/",
			wantTTL:  &timeout,
		},
		{
			name:     "uses the configured path",
			cfg:      Config{Name: "my-session", CookiePath: "/app"},
			wantName: "my-session",
			wantPath: "/app",
		},
		{
			name:     "session lifetime clears the ttl",
			cfg:      Config{Name: "my-session", AbsoluteTimeout: &timeout, LifetimeType: LifetimeSession},
			wantName: "my-session",
			wantPath: "/",
		},
		{
			name:     "permanent lifetime keeps the absolute timeout",
			cfg:      Config{Name: "my-session", AbsoluteTimeout: &timeout, LifetimeType: LifetimePermanent},
			wantName: "my-session",
			wantPath: "/",
			wantTTL:  &timeout,
		},
		{
			name:     "permanent lifetime without absolute timeout defaults the ttl",
			cfg:      Config{Name: "my-session", LifetimeType: LifetimePermanent},
			wantName: "my-session",
			wantPath: "/",
			wantTTL:  new(defaultPermanentCookieTTL),
		},
		{
			name:     "sanitizes the name",
			cfg:      Config{Name: "my session"},
			wantName: "my_session",
			wantPath: "/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CookieState(tt.cfg)
			assert.Equal(t, tt.wantName, got.GetCookie().GetName(), "cookie name")
			assert.Equal(t, tt.wantPath, got.GetCookie().GetPath(), "cookie path")
			if tt.wantTTL == nil {
				assert.Nil(t, got.GetCookie().GetTtl(), "cookie ttl should be unset")
			} else {
				assert.Equal(t, *tt.wantTTL, got.GetCookie().GetTtl().AsDuration(), "cookie ttl")
			}
		})
	}
}

func TestBuild(t *testing.T) {
	cookie, err := Build(Config{Type: Cookie, Name: "my-session"})
	require.NoError(t, err)
	assert.Equal(t, "envoy.http.stateful_session.cookie", cookie.GetStatefulSession().GetSessionState().GetName())
	cookieState := &stateful_cookie.CookieBasedSessionState{}
	require.NoError(t, cookie.GetStatefulSession().GetSessionState().GetTypedConfig().UnmarshalTo(cookieState))
	assert.Equal(t, "my-session", cookieState.GetCookie().GetName())

	header, err := Build(Config{Type: Header, Name: "x-session"})
	require.NoError(t, err)
	assert.Equal(t, "envoy.http.stateful_session.header", header.GetStatefulSession().GetSessionState().GetName())
	headerState := &stateful_header.HeaderBasedSessionState{}
	require.NoError(t, header.GetStatefulSession().GetSessionState().GetTypedConfig().UnmarshalTo(headerState))
	assert.Equal(t, "x-session", headerState.GetName())

	_, err = Build(Config{Type: "Unknown"})
	assert.EqualError(t, err, `unsupported session persistence type "Unknown"`)
}

func TestGeneratedCookieName(t *testing.T) {
	a := GeneratedCookieName("default/backend-a:3000")
	assert.Equal(t, a, GeneratedCookieName("default/backend-a:3000"), "name must be stable")
	assert.NotEqual(t, a, GeneratedCookieName("default/backend-b:3000"), "name must be unique per backend")
	assert.Regexp(t, `^kgw-sp-[0-9a-f]{16}$`, a)
	assert.NotContains(t, a, "backend-a", "name must not expose the backend name")
}

func TestDisabledFilter(t *testing.T) {
	want, err := filters.NewStagedFilter(FilterName, &stateful_sessionv3.StatefulSession{}, filters.DuringStage(filters.AcceptedStage))
	require.NoError(t, err)
	want.Filter.Disabled = true

	got := DisabledFilter()
	assert.True(t, proto.Equal(want.Filter, got.Filter), "filter should match the previously built filter")
	assert.Equal(t, want.Stage, got.Stage, "filter stage")
	assert.NotSame(t, DisabledFilter().Filter, got.Filter, "each call returns its own filter")
}

func TestConfigFromAPI(t *testing.T) {
	timeout := time.Hour
	tests := []struct {
		name    string
		in      *kgateway.SessionPersistence
		want    Config
		wantErr string
	}{
		{name: "nil", in: nil, wantErr: "session persistence is not set"},
		{
			name: "cookie defaults to a session lifetime and leaves the name unset",
			in:   &kgateway.SessionPersistence{Cookie: &kgateway.SessionPersistenceCookieConfig{}},
			want: Config{Type: Cookie, LifetimeType: LifetimeSession},
		},
		{
			name: "cookie with all fields",
			in: &kgateway.SessionPersistence{
				AbsoluteTimeout: new(gwv1.Duration("1h")),
				Cookie: &kgateway.SessionPersistenceCookieConfig{
					Name: new(kgateway.CookieName("s")), Path: new("/app"), LifetimeType: new(kgateway.PermanentCookieLifetimeType),
				},
			},
			want: Config{Type: Cookie, Name: "s", CookiePath: "/app", AbsoluteTimeout: &timeout, LifetimeType: LifetimePermanent},
		},
		{
			name: "header",
			in:   &kgateway.SessionPersistence{Type: new(kgateway.HeaderBasedSessionPersistence), Header: &kgateway.SessionPersistenceHeaderConfig{Name: "x-s"}},
			want: Config{Type: Header, Name: "x-s"},
		},
		{
			name:    "header without header config",
			in:      &kgateway.SessionPersistence{Type: new(kgateway.HeaderBasedSessionPersistence)},
			wantErr: "sessionPersistence.header must be specified for Header type",
		},
		{
			name:    "invalid timeout",
			in:      &kgateway.SessionPersistence{AbsoluteTimeout: new(gwv1.Duration("bogus")), Cookie: &kgateway.SessionPersistenceCookieConfig{}},
			wantErr: `invalid sessionPersistence.absoluteTimeout: time: invalid duration "bogus"`,
		},
		{
			name:    "unknown type",
			in:      &kgateway.SessionPersistence{Type: new(kgateway.SessionPersistenceType("Unknown"))},
			wantErr: `unsupported session persistence type "Unknown"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConfigFromAPI(tt.in)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
