package irtranslator

import (
	"context"
	"errors"
	"fmt"
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

// gatewayValidator validates configuration for one Gateway under the validation
// subset of each bootstrap the Gateway's proxies run. Every proxy of a Gateway
// receives the same xDS configuration, so configuration passes only if each of
// them accepts it.
type gatewayValidator struct {
	inner   validator.Validator
	subsets []ir.ValidationBootstrap
}

var _ validator.Validator = &gatewayValidator{}

// validatorForGateway returns v for a Gateway whose proxies all run Envoy's defaults,
// and otherwise a validator that validates under each of the Gateway's subsets.
func validatorForGateway(v validator.Validator, gw *ir.Gateway) validator.Validator {
	if v == nil || gw == nil || len(gw.ValidationBootstraps.GetSubsets()) == 0 {
		return v
	}
	return &gatewayValidator{inner: v, subsets: gw.ValidationBootstraps.GetSubsets()}
}

// Validate validates bs under each subset in turn, the current one first, and returns
// the first rejection. A rejection under the subset of a pod still running an
// earlier bootstrap names the pod.
func (g *gatewayValidator) Validate(ctx context.Context, bs *envoybootstrapv3.Bootstrap) error {
	for _, subset := range g.subsets {
		err := g.inner.Validate(ctx, withValidationSubset(bs, subset.Bootstrap))
		switch {
		case err == nil:
			continue
		case subset.Pod != "" && errors.Is(err, validator.ErrInvalidXDS):
			return fmt.Errorf("rejected by proxy pod %s, which runs an earlier bootstrap until its rollout completes: %w", subset.Pod, err)
		default:
			return err
		}
	}
	return nil
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
