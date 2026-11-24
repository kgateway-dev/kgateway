package irtranslator

import (
	"context"
	"errors"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrapoverlay"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

// raisedRE2Subset raises the RE2 program size limit from its default of 100.
const raisedRE2Subset = `{"layered_runtime":{"layers":[{"name":"limits","static_layer":{"re2.max_program_size.error_level":10000}}]}}`

func validationBootstrap(t *testing.T, subset string) ir.ValidationBootstrap {
	t.Helper()
	if subset == "" {
		return ir.ValidationBootstrap{}
	}
	bs, err := bootstrapoverlay.ParseValidationSubset(subset)
	require.NoError(t, err)
	return ir.ValidationBootstrap{Subset: subset, Bootstrap: bs}
}

func gatewayWithSubset(subset ir.ValidationBootstrap) *ir.Gateway {
	return &ir.Gateway{ValidationBootstraps: &ir.GatewayValidationBootstraps{Subset: subset}}
}

// runtimeLayerNames returns the names of bs's runtime layers.
func runtimeLayerNames(bs *envoybootstrapv3.Bootstrap) []string {
	var out []string
	for _, l := range bs.GetLayeredRuntime().GetLayers() {
		out = append(out, l.GetName())
	}
	return out
}

func TestValidatorForGatewayPassesThrough(t *testing.T) {
	inner := &routeMockValidator{}

	// A Gateway whose bootstrap runs Envoy's defaults validates with the inner validator.
	for name, gw := range map[string]*ir.Gateway{
		"no gateway":              nil,
		"no validation bootstrap": {},
		"only reserved ports":     {ValidationBootstraps: &ir.GatewayValidationBootstraps{ReservedPorts: []uint32{9000}}},
	} {
		assert.Same(t, inner, validatorForGateway(inner, gw), name)
	}

	// Without an inner validator there is nothing to wrap.
	assert.Nil(t, validatorForGateway(nil, gatewayWithSubset(validationBootstrap(t, raisedRE2Subset))))
}

func TestGatewayValidatorAddsSubset(t *testing.T) {
	// Record the runtime layers of each bootstrap the inner validator sees, and fail
	// with an error of its own.
	var seen [][]string
	rejected := errors.New("rejected")
	inner := &routeMockValidator{validateFunc: func(_ context.Context, bs *envoybootstrapv3.Bootstrap) error {
		seen = append(seen, runtimeLayerNames(bs))
		return rejected
	}}
	v := validatorForGateway(inner, gatewayWithSubset(validationBootstrap(t, raisedRE2Subset)))

	// Validate a bootstrap that has a layer of its own.
	bs := &envoybootstrapv3.Bootstrap{LayeredRuntime: &envoybootstrapv3.LayeredRuntime{
		Layers: []*envoybootstrapv3.RuntimeLayer{{Name: "base"}},
	}}
	before := proto.CloneOf(bs)
	err := v.Validate(t.Context(), bs)

	// The inner validator ran once, with the subset's layers after the bootstrap's own,
	// and its error is returned as is.
	assert.Equal(t, [][]string{{"base", "limits"}}, seen)
	assert.Same(t, rejected, err)
	assert.True(t, proto.Equal(before, bs), "adding a subset must not modify the bootstrap under validation")
}

// A static runtime layer in the bootstrap changes which routes Envoy accepts, so
// validation must run under the subset of the Gateway's current bootstrap.
func TestGatewayValidatorWithEnvoy(t *testing.T) {
	// A route whose regex compiles to more than RE2's default program size limit.
	route := newRouteWithPrefix("/")
	route.Match = &envoyroutev3.RouteMatch{PathSpecifier: &envoyroutev3.RouteMatch_SafeRegex{
		SafeRegex: &envoy_type_matcher_v3.RegexMatcher{Regex: "/(abcdefgh){20}"},
	}}
	envoy := validator.NewDocker()
	validate := func(gw *ir.Gateway) error {
		return validateFullRoutes(t.Context(), []*envoyroutev3.Route{route}, validatorForGateway(envoy, gw))
	}

	// Envoy's defaults reject the route.
	err := validate(nil)
	require.ErrorIs(t, err, validator.ErrInvalidXDS)
	assert.Contains(t, err.Error(), "program size")

	// A Gateway whose bootstrap raises the limit accepts it.
	require.NoError(t, validate(gatewayWithSubset(validationBootstrap(t, raisedRE2Subset))))

	// A lowered limit rejects a short regex that Envoy's defaults accept.
	short := newRouteWithPrefix("/")
	short.Match = &envoyroutev3.RouteMatch{PathSpecifier: &envoyroutev3.RouteMatch_SafeRegex{
		SafeRegex: &envoy_type_matcher_v3.RegexMatcher{Regex: "/abcdefghijklmnop"},
	}}
	require.NoError(t, validateFullRoutes(t.Context(), []*envoyroutev3.Route{short}, envoy))
	err = validateFullRoutes(t.Context(), []*envoyroutev3.Route{short}, validatorForGateway(envoy, gatewayWithSubset(
		validationBootstrap(t, `{"layered_runtime":{"layers":[{"name":"limits","static_layer":{"re2.max_program_size.error_level":10}}]}}`),
	)))
	require.ErrorIs(t, err, validator.ErrInvalidXDS)
}
