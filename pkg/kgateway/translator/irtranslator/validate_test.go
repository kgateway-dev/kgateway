package irtranslator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		{name: "path with fragment only no query", path: "/test", wantErr: false},
		{name: "invalid path sequence", path: "//test", wantErr: true},
		{name: "invalid path with query", path: "/test/../foo?bar=baz", wantErr: true},
		{name: "path with hash after query", path: "/test?ref=val#section", wantErr: false},
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
	err := validateRoute(context.Background(), newRouteWithRedirectPath("//invalid-path"), v, apisettings.ValidationStandard)
	require.Error(t, err, "redirect path with double slash should be invalid")
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
