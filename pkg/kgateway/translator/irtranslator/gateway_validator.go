package irtranslator

import (
	"context"
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

// gatewayValidator validates configuration for one Gateway under the validation
// subset of the Gateway's current bootstrap, the one new proxy pods start with.
type gatewayValidator struct {
	inner  validator.Validator
	subset *envoybootstrapv3.Bootstrap
}

var _ validator.Validator = &gatewayValidator{}

// validatorForGateway returns v for a Gateway whose bootstrap runs Envoy's defaults,
// and otherwise a validator that validates under the Gateway's subset.
func validatorForGateway(v validator.Validator, gw *ir.Gateway) validator.Validator {
	if v == nil || gw == nil {
		return v
	}
	subset := gw.ValidationBootstraps.GetSubset()
	if subset.Subset == "" {
		return v
	}
	return &gatewayValidator{inner: v, subset: subset.Bootstrap}
}

// Validate validates bs under the Gateway's subset.
func (g *gatewayValidator) Validate(ctx context.Context, bs *envoybootstrapv3.Bootstrap) error {
	return g.inner.Validate(ctx, withValidationSubset(bs, g.subset))
}

// withValidationSubset returns bs with the runtime layers of subset added after its
// own, without modifying bs. With a subset that has no layers it returns bs.
func withValidationSubset(bs, subset *envoybootstrapv3.Bootstrap) *envoybootstrapv3.Bootstrap {
	layers := subset.GetLayeredRuntime().GetLayers()
	if len(layers) == 0 {
		return bs
	}

	// Share bs's other fields rather than copying the configuration under validation.
	out := cloneWithoutField(bs, "layered_runtime")
	out.LayeredRuntime = &envoybootstrapv3.LayeredRuntime{
		Layers: slices.Concat(bs.GetLayeredRuntime().GetLayers(), layers),
	}
	return out
}
