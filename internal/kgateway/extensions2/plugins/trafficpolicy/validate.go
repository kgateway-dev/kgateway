package trafficpolicy

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
	"github.com/kgateway-dev/kgateway/v2/pkg/xds/bootstrap"
)

// validateWithValidationLevel performs validation based on validation level.
// Callers who need validation level behavior should use this method instead of calling
// the Validate() method on the TrafficPolicy type directly.
func validateWithValidationLevel(ctx context.Context, p *TrafficPolicy, v validator.Validator, mode apisettings.ValidationMode) error {
	switch mode {
	case apisettings.ValidationStandard:
		return p.Validate()
	case apisettings.ValidationStrict:
		if err := p.Validate(); err != nil {
			return err
		}
		return validateXDS(ctx, p, v)
	}
	return nil
}

// validateXDS performs only xDS validation by building a partial bootstrap config and validating
// it via envoy validate mode. It re-uses the ApplyForRoute method to ensure that the translation
// and validation logic go through the same code path as normal.
// This method can be called independently when only xDS validation is needed.
func validateXDS(ctx context.Context, p *TrafficPolicy, v validator.Validator) error {
	// use a fake translation pass to ensure we have the desired typed filter config
	// on the placeholder vhost.
	typedPerFilterConfig := ir.TypedFilterConfigMap(map[string]proto.Message{})
	fakePass := NewGatewayTranslationPass(ir.GwTranslationCtx{}, nil).(*trafficPolicyPluginGwPass)

	// Use a placeholder filter chain name for validation
	const validationFilterChain = "validation-filter-chain"

	if err := fakePass.ApplyForRoute(&ir.RouteContext{
		Policy:            p,
		TypedFilterConfig: typedPerFilterConfig,
		FilterChainName:   validationFilterChain,
	}, nil); err != nil {
		return err
	}

	// build a partial bootstrap config with the typed filter config applied.
	builder := bootstrap.New()
	for name, config := range typedPerFilterConfig {
		builder.AddFilterConfig(name, config)
	}

	// Get HTTP filters from the translation pass and add to builder.
	// This ensures that HTTP filter configurations (like ext_authz with invalid pathPrefix)
	// are validated by Envoy.
	fcc := ir.FilterChainCommon{FilterChainName: validationFilterChain}
	httpFilters, err := fakePass.HttpFilters(fcc)
	if err != nil {
		return fmt.Errorf("failed to get HTTP filters for validation: %w", err)
	}
	for _, stagedFilter := range httpFilters {
		builder.AddHttpFilter(stagedFilter.Filter)
	}

	bootstrapCfg, err := builder.Build()
	if err != nil {
		return err
	}
	data, err := protojson.Marshal(bootstrapCfg)
	if err != nil {
		return err
	}

	// shell out to envoy to validate the partial bootstrap config.
	return v.Validate(ctx, string(data))
}
