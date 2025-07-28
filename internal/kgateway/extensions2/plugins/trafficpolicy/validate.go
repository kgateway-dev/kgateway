package trafficpolicy

import (
	"context"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
	"github.com/kgateway-dev/kgateway/v2/pkg/xds/bootstrap"
)

func (p *TrafficPolicy) Validate(ctx context.Context, v validator.Validator, mode settings.RouteReplacementMode) error {
	switch mode {
	case settings.RouteReplacementStandard:
		return p.validateStandard()
	case settings.RouteReplacementStrict:
		return p.validateStrict(ctx, v)
	}
	return nil
}

// validateStandard performs basic proto validation that runs in STANDARD mode
func (p *TrafficPolicy) validateStandard() error {
	return p.validateProto()
}

// validateStrict performs both proto and xDS validation that runs in STRICT mode
func (p *TrafficPolicy) validateStrict(ctx context.Context, v validator.Validator) error {
	if err := p.validateStandard(); err != nil {
		return err
	}
	return p.validateXDS(ctx, v)
}

// validateProto performs basic proto validation that runs in STANDARD mode.
// TODO(tim): this logic will be refactored in the future to be less brittle,
// easier to read/maintain/etc. but requires additional traffic policy plugin
// refactoring to do properly.
func (p *TrafficPolicy) validateProto() error {
	// TODO: rustformations, and ext auth/rate limit provider validation
	// Note: no need for buffer validation as it's a single int field, right?
	var validators []func() error
	if p.spec.ai != nil {
		if p.spec.ai.Transformation != nil {
			validators = append(validators, p.spec.ai.Transformation.Validate)
		}
		if p.spec.ai.Extproc != nil {
			validators = append(validators, p.spec.ai.Extproc.Validate)
		}
	}
	if p.spec.transformation != nil && p.spec.transformation.config != nil {
		validators = append(validators, p.spec.transformation.config.Validate)
	}
	if p.spec.localRateLimit != nil && p.spec.localRateLimit.config != nil {
		validators = append(validators, p.spec.localRateLimit.config.Validate)
	}
	if p.spec.globalRateLimit != nil {
		for _, rateLimit := range p.spec.globalRateLimit.rateLimitActions {
			validators = append(validators, rateLimit.Validate)
		}
	}
	if p.spec.extProc != nil {
		if p.spec.extProc.perRoute != nil {
			validators = append(validators, p.spec.extProc.perRoute.Validate)
		}
	}
	if p.spec.extAuth != nil {
		if p.spec.extAuth.perRoute != nil {
			validators = append(validators, p.spec.extAuth.perRoute.Validate)
		}
	}
	if p.spec.csrf != nil {
		validators = append(validators, p.spec.csrf.policy.Validate)
	}
	for _, validator := range validators {
		if err := validator(); err != nil {
			return err
		}
	}
	return nil
}

// validateXDS builds a partial bootstrap config and validates it via envoy
// validate mode. It re-uses the ApplyForRoute method to ensure that the translation
// and validation logic go through the same code path as normal.
func (p *TrafficPolicy) validateXDS(ctx context.Context, v validator.Validator) error {
	// use a fake translation pass to ensure we have the desired typed filter config
	// on the placeholder vhost.
	typedPerFilterConfig := ir.TypedFilterConfigMap(map[string]proto.Message{})
	fakePass := NewGatewayTranslationPass(ctx, ir.GwTranslationCtx{}, nil)
	if err := fakePass.ApplyForRoute(ctx, &ir.RouteContext{
		Policy:            p,
		TypedFilterConfig: typedPerFilterConfig,
	}, nil); err != nil {
		return err
	}

	// build a partial bootstrap config with the typed filter config applied.
	builder := bootstrap.New()
	for name, config := range typedPerFilterConfig {
		builder.AddFilterConfig(name, config)
	}
	bootstrap, err := builder.Build()
	if err != nil {
		return err
	}
	data, err := protojson.Marshal(bootstrap)
	if err != nil {
		return err
	}

	// shell out to envoy to validate the partial bootstrap config.
	return v.Validate(ctx, string(data))
}
