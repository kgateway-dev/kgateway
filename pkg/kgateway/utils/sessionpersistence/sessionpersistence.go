// Package sessionpersistence converts session persistence settings into the
// Envoy stateful_session filter configuration. It is shared by the route level
// (HTTPRoute/GRPCRoute rule) and BackendConfigPolicy implementations so both
// produce identical Envoy config.
package sessionpersistence

import (
	"errors"
	"fmt"
	"strings"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	stateful_sessionv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/stateful_session/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	stateful_cookie "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/cookie/v3"
	stateful_header "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/header/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/http/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
)

// FilterName is the name of the Envoy stateful session HTTP filter.
const FilterName = "envoy.filters.http.stateful_session"

// Type is the session persistence type.
type Type string

const (
	Cookie Type = "Cookie"
	Header Type = "Header"
)

// LifetimeType is the lifetime of a session persistence cookie.
type LifetimeType string

const (
	LifetimeSession   LifetimeType = "Session"
	LifetimePermanent LifetimeType = "Permanent"
)

// defaultPermanentCookieTTL is used for Permanent cookies without an AbsoluteTimeout.
const defaultPermanentCookieTTL = time.Hour * 24 * 365

// Config is the API neutral session persistence configuration.
type Config struct {
	Type Type
	// Name is the cookie or header name. It is sanitized.
	Name string
	// CookiePath is the cookie Path attribute. Defaults to "/".
	CookiePath      string
	AbsoluteTimeout *time.Duration
	LifetimeType    LifetimeType
}

// ConfigFromAPI converts the kgateway SessionPersistence API into a Config. It is shared by
// BackendConfigPolicy and the route rule sessionPersistence (converted into the kgateway API first).
// An unset cookie name is returned as an empty Name so the caller can choose a default.
func ConfigFromAPI(in *kgateway.SessionPersistence) (Config, error) {
	if in == nil {
		return Config{}, errors.New("session persistence is not set")
	}
	cfg := Config{
		Type: Type(ptr.Deref(in.Type, kgateway.CookieBasedSessionPersistence)),
	}
	if in.AbsoluteTimeout != nil {
		d, err := time.ParseDuration(string(*in.AbsoluteTimeout))
		if err != nil {
			return Config{}, fmt.Errorf("invalid sessionPersistence.absoluteTimeout: %w", err)
		}
		cfg.AbsoluteTimeout = &d
	}
	switch cfg.Type {
	case Cookie:
		cfg.LifetimeType = LifetimeSession
		if c := in.Cookie; c != nil {
			cfg.Name = string(ptr.Deref(c.Name, ""))
			cfg.CookiePath = ptr.Deref(c.Path, "")
			cfg.LifetimeType = LifetimeType(ptr.Deref(c.LifetimeType, kgateway.SessionCookieLifetimeType))
		}
	case Header:
		if in.Header == nil {
			return Config{}, errors.New("sessionPersistence.header must be specified for Header type")
		}
		cfg.Name = string(in.Header.Name)
	default:
		return Config{}, fmt.Errorf("unsupported session persistence type %q", cfg.Type)
	}
	return cfg, nil
}

// CookieState builds the Envoy cookie based session state for cfg.
func CookieState(cfg Config) *stateful_cookie.CookieBasedSessionState {
	var ttl *durationpb.Duration
	if cfg.AbsoluteTimeout != nil {
		ttl = durationpb.New(*cfg.AbsoluteTimeout)
	}
	switch cfg.LifetimeType {
	case LifetimeSession:
		// Session cookies (cookies without a Max-Age or Expires attribute) are deleted when the current session ends
		ttl = nil
	case LifetimePermanent:
		if ttl == nil {
			ttl = durationpb.New(defaultPermanentCookieTTL)
		}
	}
	path := cfg.CookiePath
	if path == "" {
		path = "/"
	}
	return &stateful_cookie.CookieBasedSessionState{
		Cookie: &httpv3.Cookie{
			Name: utils.SanitizeCookieName(cfg.Name),
			Ttl:  ttl,
			Path: path,
		},
	}
}

// HeaderState builds the Envoy header based session state for cfg.
func HeaderState(cfg Config) *stateful_header.HeaderBasedSessionState {
	return &stateful_header.HeaderBasedSessionState{
		Name: utils.SanitizeHeaderName(cfg.Name),
	}
}

// Build converts cfg into the per route config of the stateful session filter.
func Build(cfg Config) (*stateful_sessionv3.StatefulSessionPerRoute, error) {
	switch cfg.Type {
	case Cookie:
		return PerRoute(CookieState(cfg))
	case Header:
		return PerRoute(HeaderState(cfg))
	default:
		return nil, fmt.Errorf("unsupported session persistence type %q", cfg.Type)
	}
}

// PerRoute wraps a cookie or header session state into the per route config of
// the stateful session filter.
func PerRoute(state proto.Message) (*stateful_sessionv3.StatefulSessionPerRoute, error) {
	var spType Type
	switch state.(type) {
	case *stateful_cookie.CookieBasedSessionState:
		spType = Cookie
	case *stateful_header.HeaderBasedSessionState:
		spType = Header
	default:
		return nil, fmt.Errorf("unsupported session state type %T", state)
	}
	stateAny, err := utils.MessageToAny(state)
	if err != nil {
		return nil, err
	}
	return &stateful_sessionv3.StatefulSessionPerRoute{
		Override: &stateful_sessionv3.StatefulSessionPerRoute_StatefulSession{
			StatefulSession: &stateful_sessionv3.StatefulSession{
				SessionState: &envoycorev3.TypedExtensionConfig{
					Name:        "envoy.http.stateful_session." + strings.ToLower(string(spType)),
					TypedConfig: stateAny,
				},
			},
		},
	}, nil
}

// disabledFilterConfig is the empty stateful_session filter config, marshaled once.
// The per route config enables and configures the filter.
var disabledFilterConfig = utils.MustMessageToAny(&stateful_sessionv3.StatefulSession{})

// DisabledFilter returns the stateful_session HTTP filter, disabled by default so it only
// applies to routes that enable it with a per route config. The returned filter shares its
// marshaled config with other callers, which must not modify it.
func DisabledFilter() filters.StagedHttpFilter {
	return filters.StagedHttpFilter{
		Filter: &envoyhttp.HttpFilter{
			Name:     FilterName,
			Disabled: true,
			ConfigType: &envoyhttp.HttpFilter_TypedConfig{
				TypedConfig: disabledFilterConfig,
			},
		},
		Stage: filters.DuringStage(filters.AcceptedStage),
	}
}

// GeneratedCookieName returns a stable cookie name that is unique per backend.
// It hashes the backend identity so the cookie does not expose internal names.
func GeneratedCookieName(backendID string) string {
	return fmt.Sprintf("kgw-sp-%016x", utils.HashString(backendID))
}

// SessionKey returns the type and cookie or header name used by perRoute, for
// example "Cookie/my-session". It returns "" if perRoute has no session state.
func SessionKey(perRoute *stateful_sessionv3.StatefulSessionPerRoute) string {
	state := perRoute.GetStatefulSession().GetSessionState().GetTypedConfig()
	if state == nil {
		return ""
	}
	msg, err := state.UnmarshalNew()
	if err != nil {
		return ""
	}
	switch s := msg.(type) {
	case *stateful_cookie.CookieBasedSessionState:
		return string(Cookie) + "/" + s.GetCookie().GetName()
	case *stateful_header.HeaderBasedSessionState:
		return string(Header) + "/" + s.GetName()
	}
	return ""
}
