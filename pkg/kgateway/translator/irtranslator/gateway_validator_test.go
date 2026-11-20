package irtranslator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

// raisedRE2Subset raises the RE2 program size limit from its default of 100.
const raisedRE2Subset = `{"layered_runtime":{"layers":[{"name":"limits","static_layer":{"re2.max_program_size.error_level":10000}}]}}`

func validationBootstrap(t *testing.T, subset, pod string) ir.ValidationBootstrap {
	t.Helper()
	if subset == "" {
		return ir.ValidationBootstrap{Pod: pod}
	}
	bs, err := bootstrappatch.ParseValidationSubset(subset)
	require.NoError(t, err)
	return ir.ValidationBootstrap{Subset: subset, Pod: pod, Bootstrap: bs}
}

func gatewayWithSubsets(subsets ...ir.ValidationBootstrap) *ir.Gateway {
	return &ir.Gateway{ValidationBootstraps: &ir.GatewayValidationBootstraps{Subsets: subsets}}
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

	// A Gateway whose proxies all run Envoy's defaults validates with the inner validator.
	for name, gw := range map[string]*ir.Gateway{
		"no gateway":              nil,
		"no validation bootstrap": {},
		"only reserved ports":     {ValidationBootstraps: &ir.GatewayValidationBootstraps{ReservedPorts: []uint32{9000}}},
	} {
		assert.Same(t, inner, validatorForGateway(inner, gw), name)
	}

	// Without an inner validator there is nothing to wrap.
	assert.Nil(t, validatorForGateway(nil, gatewayWithSubsets(validationBootstrap(t, raisedRE2Subset, ""))))
}

func TestGatewayValidatorValidatesUnderEachSubset(t *testing.T) {
	// Record the runtime layers of each bootstrap the inner validator sees.
	var seen [][]string
	inner := &routeMockValidator{validateFunc: func(_ context.Context, bs *envoybootstrapv3.Bootstrap) error {
		seen = append(seen, runtimeLayerNames(bs))
		return nil
	}}
	v := validatorForGateway(inner, gatewayWithSubsets(
		validationBootstrap(t, raisedRE2Subset, ""),
		validationBootstrap(t, "", "proxy-old"),
	))

	// Validate a bootstrap that has a layer of its own.
	bs := &envoybootstrapv3.Bootstrap{LayeredRuntime: &envoybootstrapv3.LayeredRuntime{
		Layers: []*envoybootstrapv3.RuntimeLayer{{Name: "base"}},
	}}
	before := proto.CloneOf(bs)
	require.NoError(t, v.Validate(t.Context(), bs))

	// The current subset's layers follow the bootstrap's own, and the pod with Envoy's
	// defaults sees the bootstrap unchanged.
	assert.Equal(t, [][]string{{"base", "limits"}, {"base"}}, seen)
	assert.True(t, proto.Equal(before, bs), "adding a subset must not modify the bootstrap under validation")
}

func TestGatewayValidatorNamesPod(t *testing.T) {
	// Reject every bootstrap without the raised limit.
	inner := &routeMockValidator{validateFunc: func(_ context.Context, bs *envoybootstrapv3.Bootstrap) error {
		if len(bs.GetLayeredRuntime().GetLayers()) == 0 {
			return fmt.Errorf("%w: RE2 program size of 205 > max program size of 100", validator.ErrInvalidXDS)
		}
		return nil
	}}

	// A rejection under the current subset does not name a pod.
	err := validatorForGateway(inner, gatewayWithSubsets(validationBootstrap(t, "", ""))).
		Validate(t.Context(), &envoybootstrapv3.Bootstrap{})
	require.ErrorIs(t, err, validator.ErrInvalidXDS)
	assert.NotContains(t, err.Error(), "proxy pod")

	// A rejection only under a running pod's subset names the pod.
	err = validatorForGateway(inner, gatewayWithSubsets(
		validationBootstrap(t, raisedRE2Subset, ""),
		validationBootstrap(t, "", "proxy-old"),
	)).Validate(t.Context(), &envoybootstrapv3.Bootstrap{})
	require.ErrorIs(t, err, validator.ErrInvalidXDS)
	assert.Contains(t, err.Error(), "rejected by proxy pod proxy-old, which runs an earlier bootstrap until its rollout completes")

	// A failure to run Envoy is not attributed to a pod.
	transient := errors.New("envoy validate invocation failed")
	err = validatorForGateway(&routeMockValidator{validateFunc: func(_ context.Context, bs *envoybootstrapv3.Bootstrap) error {
		if len(bs.GetLayeredRuntime().GetLayers()) == 0 {
			return transient
		}
		return nil
	}}, gatewayWithSubsets(
		validationBootstrap(t, raisedRE2Subset, ""),
		validationBootstrap(t, "", "proxy-old"),
	)).Validate(t.Context(), &envoybootstrapv3.Bootstrap{})
	assert.Same(t, transient, err)
}

// A static runtime layer in the bootstrap changes which routes Envoy accepts, so
// validation must run under the subset of each bootstrap the Gateway's proxies run.
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

	// A Gateway whose proxies all run the raised limit accepts it.
	require.NoError(t, validate(gatewayWithSubsets(validationBootstrap(t, raisedRE2Subset, ""))))

	// While a pod still runs Envoy's defaults, the route is rejected, naming the pod.
	err = validate(gatewayWithSubsets(
		validationBootstrap(t, raisedRE2Subset, ""),
		validationBootstrap(t, "", "proxy-old"),
	))
	require.ErrorIs(t, err, validator.ErrInvalidXDS)
	assert.Contains(t, err.Error(), "proxy pod proxy-old")

	// A lowered limit rejects a short regex that Envoy's defaults accept, as soon as
	// the current bootstrap has it.
	short := newRouteWithPrefix("/")
	short.Match = &envoyroutev3.RouteMatch{PathSpecifier: &envoyroutev3.RouteMatch_SafeRegex{
		SafeRegex: &envoy_type_matcher_v3.RegexMatcher{Regex: "/abcdefghijklmnop"},
	}}
	require.NoError(t, validateFullRoutes(t.Context(), []*envoyroutev3.Route{short}, envoy))
	err = validateFullRoutes(t.Context(), []*envoyroutev3.Route{short}, validatorForGateway(envoy, gatewayWithSubsets(
		validationBootstrap(t, `{"layered_runtime":{"layers":[{"name":"limits","static_layer":{"re2.max_program_size.error_level":10}}]}}`, ""),
		validationBootstrap(t, "", "proxy-old"),
	)))
	require.ErrorIs(t, err, validator.ErrInvalidXDS)
	assert.NotContains(t, err.Error(), "proxy pod", "the current bootstrap rejects the route, not the running pod")
}
