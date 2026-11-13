package irtranslator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

type countingValidator struct {
	calls       atomic.Int32
	failureFunc func(call int) error
}

func (c *countingValidator) Validate(_ context.Context, _ *envoybootstrapv3.Bootstrap) error {
	n := c.calls.Add(1)
	if c.failureFunc == nil {
		return nil
	}
	return c.failureFunc(int(n))
}

func newRouteWithPrefix(prefix string) *envoyroutev3.Route {
	return &envoyroutev3.Route{
		Name: "r",
		Match: &envoyroutev3.RouteMatch{
			PathSpecifier: &envoyroutev3.RouteMatch_Prefix{Prefix: prefix},
		},
		Action: &envoyroutev3.Route_Route{
			Route: &envoyroutev3.RouteAction{
				ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "c"},
			},
		},
	}
}

func TestValidateRoute_NilRoute(t *testing.T) {
	err := validateRoute(context.Background(), nil, &countingValidator{}, apisettings.ValidationStrict)
	require.Error(t, err)
}

func TestValidateRoute_StaticFailureIsRouteError(t *testing.T) {
	v := &countingValidator{}
	err := validateRoute(context.Background(), newRouteWithPrefix("//bad"), v, apisettings.ValidationStrict)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRoute))
	assert.False(t, errors.Is(err, ErrInvalidMatcher))
	assert.Zero(t, v.calls.Load(), "static failure must not invoke the validator")
}

func TestValidateRoute_StandardModeSkipsValidator(t *testing.T) {
	v := &countingValidator{}
	err := validateRoute(context.Background(), newRouteWithPrefix("/ok"), v, apisettings.ValidationStandard)
	require.NoError(t, err)
	assert.Zero(t, v.calls.Load(), "standard mode must not invoke the validator")
}

func TestValidateRoute_StrictValidSingleCall(t *testing.T) {
	v := &countingValidator{}
	err := validateRoute(context.Background(), newRouteWithPrefix("/ok"), v, apisettings.ValidationStrict)
	require.NoError(t, err)
	assert.Equal(t, int32(1), v.calls.Load(), "valid strict route must invoke validator exactly once")
}

func TestValidateRoute_StrictRouteActionFailure(t *testing.T) {
	v := &countingValidator{
		failureFunc: func(call int) error {
			if call == 1 {
				return errors.New("cluster reference broken")
			}
			return nil
		},
	}
	err := validateRoute(context.Background(), newRouteWithPrefix("/ok"), v, apisettings.ValidationStrict)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRoute))
	assert.False(t, errors.Is(err, ErrInvalidMatcher))
	assert.Equal(t, int32(2), v.calls.Load(), "route-action failure pays the disambiguation call")
}

func TestValidateRedirectPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "empty path", path: "", wantErr: false},
		{name: "simple path", path: "/test", wantErr: false},
		{name: "path with query params", path: "/test?foo=bar", wantErr: false},
		{name: "path with multiple query params", path: "/test?foo=bar&baz=qux", wantErr: false},
		{name: "path with encoded chars", path: "/test?redirect=%2Ffoo", wantErr: false},
		{name: "path with empty query", path: "/test?", wantErr: false},
		{name: "path with fragment only no query", path: "/test#section", wantErr: false},
		{name: "path with hash after query", path: "/test?ref=val#section", wantErr: false},
		{name: "query with brackets", path: "/test?foo[]=1", wantErr: false},
		{name: "query with non-ascii byte", path: "/test?foo=\u00e9", wantErr: false},
		// A redirect target is never matched on, so the sequences ValidateRoutePath rejects for
		// match paths are all legal here. Envoy accepts them, so kgateway must not be stricter.
		{name: "double slash", path: "//test", wantErr: false},
		{name: "dot dot segment", path: "/test/../foo?bar=baz", wantErr: false},
		{name: "encoded slash", path: "/test%2Ffoo", wantErr: false},
		{name: "trailing dot dot", path: "/test/..", wantErr: false},
		// Envoy's own path_redirect constraint is ^[^\x00\n\r]*$, and violating it makes Envoy
		// reject the whole RouteConfiguration, so catch it before it is sent.
		{name: "CRLF in query", path: "/test?foo=bar\r\nx-injected: 1", wantErr: true},
		{name: "CRLF in fragment", path: "/test#frag\r\nx-injected: 1", wantErr: true},
		{name: "bare LF", path: "/test\nfoo", wantErr: true},
		{name: "NUL", path: "/test\x00", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errs []error
			validateRedirectPath(tt.path, &errs)
			if tt.wantErr {
				require.NotEmpty(t, errs, "expected error for path %q", tt.path)
			} else {
				require.Empty(t, errs, "unexpected error for path %q: %v", tt.path, errs)
			}
		})
	}
}

func newRouteWithRedirectPath(redirectPath string) *envoyroutev3.Route {
	return &envoyroutev3.Route{
		Name: "r",
		Match: &envoyroutev3.RouteMatch{
			PathSpecifier: &envoyroutev3.RouteMatch_Path{Path: "/original"},
		},
		Action: &envoyroutev3.Route_Redirect{
			Redirect: &envoyroutev3.RedirectAction{
				PathRewriteSpecifier: &envoyroutev3.RedirectAction_PathRedirect{
					PathRedirect: redirectPath,
				},
			},
		},
	}
}

func TestValidateRoute_RedirectPathWithQueryParams(t *testing.T) {
	v := &countingValidator{}
	err := validateRoute(context.Background(), newRouteWithRedirectPath("/redirect?foo=bar"), v, apisettings.ValidationStandard)
	require.NoError(t, err, "redirect path with query params should be valid")
}

func TestValidateRoute_RedirectPathWithInvalidPath(t *testing.T) {
	v := &countingValidator{}
	err := validateRoute(context.Background(), newRouteWithRedirectPath("/redirect\r\nx-injected: 1"), v, apisettings.ValidationStandard)
	require.Error(t, err, "redirect path with CRLF should be invalid")
}

func TestValidateRoute_StrictMatcherFailure(t *testing.T) {
	v := &countingValidator{
		failureFunc: func(call int) error {
			return errors.New("matcher rejected")
		},
	}
	err := validateRoute(context.Background(), newRouteWithPrefix("/ok"), v, apisettings.ValidationStrict)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidMatcher))
	assert.False(t, errors.Is(err, ErrInvalidRoute))
	assert.Equal(t, int32(2), v.calls.Load())
}

// Host and pseudo-header mutations NACK the entire RouteConfiguration in Envoy.
// Validate both route-level and backendRef filters before publishing it.
func TestValidateRoute_HeaderMutations(t *testing.T) {
	for _, mode := range []apisettings.ValidationMode{apisettings.ValidationStandard, apisettings.ValidationStrict} {
		for _, backend := range []bool{false, true} {
			for _, response := range []bool{false, true} {
				for _, operation := range []string{"add", "set", "remove"} {
					for _, name := range []string{"host", "Host", "hOsT", ":authority", ":path", ":status", "X-Forwarded-Host", "X-Test"} {
						t.Run(fmt.Sprintf("%s/backend=%t/response=%t/%s/%s", mode, backend, response, operation, name), func(t *testing.T) {
							r := newRouteWithPrefix("/")
							r.GetRoute().HostRewriteSpecifier = &envoyroutev3.RouteAction_HostRewriteLiteral{HostRewriteLiteral: "backend.example.com"}
							var add []*envoycorev3.HeaderValueOption
							var remove []string
							if operation == "remove" {
								remove = []string{name}
							} else {
								action := envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD
								if operation == "set" {
									action = envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD
								}
								add = []*envoycorev3.HeaderValueOption{{Header: &envoycorev3.HeaderValue{Key: name, Value: "api.example.com"}, AppendAction: action}}
							}
							if backend {
								c := &envoyroutev3.WeightedCluster_ClusterWeight{Name: "c", Weight: wrapperspb.UInt32(1)}
								if response {
									c.ResponseHeadersToAdd, c.ResponseHeadersToRemove = add, remove
								} else {
									c.RequestHeadersToAdd, c.RequestHeadersToRemove = add, remove
								}
								r.GetRoute().ClusterSpecifier = &envoyroutev3.RouteAction_WeightedClusters{WeightedClusters: &envoyroutev3.WeightedCluster{Clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{c}}}
							} else if response {
								r.ResponseHeadersToAdd, r.ResponseHeadersToRemove = add, remove
							} else {
								r.RequestHeadersToAdd, r.RequestHeadersToRemove = add, remove
							}
							v := &countingValidator{}
							err := validateRoute(context.Background(), r, v, mode)
							if strings.HasPrefix(name, "X-") {
								require.NoError(t, err)
							} else {
								require.ErrorIs(t, err, ErrInvalidRoute)
								require.NotErrorIs(t, err, ErrInvalidMatcher)
								require.ErrorContains(t, err, name)
								require.Zero(t, v.calls.Load(), "invalid headers must be caught without invoking Envoy")
							}
						})
					}
				}
			}
		}
	}
}

// A header value Envoy cannot parse as a substitution format NACKs the entire
// RouteConfiguration, as does a host rewrite containing CR or LF. Catch both in
// standard mode on the route and on each weighted cluster.
func TestValidateRoute_HeaderValuesAndHostRewrite(t *testing.T) {
	for _, mode := range []apisettings.ValidationMode{apisettings.ValidationStandard, apisettings.ValidationStrict} {
		for _, backend := range []bool{false, true} {
			for _, response := range []bool{false, true} {
				for value, valid := range map[string]bool{
					"%REQ(x-id)%":     true,
					"100%%":           true,
					"%REQ(x-id":       false,
					"%RESPONSE(x-id)": false,
					"100%":            false,
					"a\r\nb: c":       false,
				} {
					t.Run(fmt.Sprintf("%s/backend=%t/response=%t/%q", mode, backend, response, value), func(t *testing.T) {
						r := newRouteWithPrefix("/")
						add := []*envoycorev3.HeaderValueOption{{Header: &envoycorev3.HeaderValue{Key: "x-test", Value: value}}}
						if backend {
							c := &envoyroutev3.WeightedCluster_ClusterWeight{Name: "c", Weight: wrapperspb.UInt32(1)}
							if response {
								c.ResponseHeadersToAdd = add
							} else {
								c.RequestHeadersToAdd = add
							}
							r.GetRoute().ClusterSpecifier = &envoyroutev3.RouteAction_WeightedClusters{WeightedClusters: &envoyroutev3.WeightedCluster{Clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{c}}}
						} else if response {
							r.ResponseHeadersToAdd = add
						} else {
							r.RequestHeadersToAdd = add
						}
						v := &countingValidator{}
						err := validateRoute(context.Background(), r, v, mode)
						if valid {
							require.NoError(t, err)
							return
						}
						require.ErrorIs(t, err, ErrInvalidRoute)
						require.ErrorContains(t, err, `invalid value for header "x-test"`)
						require.Zero(t, v.calls.Load(), "invalid values must be caught without invoking Envoy")
					})
				}
			}
		}
	}

	for _, backend := range []bool{false, true} {
		for host, valid := range map[string]bool{"api.example.com": true, "100%": true, "a\r\nb": false} {
			t.Run(fmt.Sprintf("host rewrite/backend=%t/%q", backend, host), func(t *testing.T) {
				r := newRouteWithPrefix("/")
				if backend {
					c := &envoyroutev3.WeightedCluster_ClusterWeight{
						Name: "c", Weight: wrapperspb.UInt32(1),
						HostRewriteSpecifier: &envoyroutev3.WeightedCluster_ClusterWeight_HostRewriteLiteral{HostRewriteLiteral: host},
					}
					r.GetRoute().ClusterSpecifier = &envoyroutev3.RouteAction_WeightedClusters{WeightedClusters: &envoyroutev3.WeightedCluster{Clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{c}}}
				} else {
					r.GetRoute().HostRewriteSpecifier = &envoyroutev3.RouteAction_HostRewriteLiteral{HostRewriteLiteral: host}
				}
				err := validateRoute(context.Background(), r, &countingValidator{}, apisettings.ValidationStandard)
				if valid {
					require.NoError(t, err)
					return
				}
				require.ErrorIs(t, err, ErrInvalidRoute)
				require.ErrorContains(t, err, "invalid host rewrite")
			})
		}
	}
}
