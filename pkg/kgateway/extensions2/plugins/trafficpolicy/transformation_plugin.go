package trafficpolicy

import (
	"encoding/json"

	extensiondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

type rustformationIR struct {
	config *dynamicmodulesv3.DynamicModuleFilterPerRoute
}

var _ PolicySubIR = &rustformationIR{}

func (r *rustformationIR) Equals(other PolicySubIR) bool {
	otherRustformation, ok := other.(*rustformationIR)
	if !ok {
		return false
	}
	if r == nil && otherRustformation == nil {
		return true
	}
	if r == nil || otherRustformation == nil {
		return false
	}
	return proto.Equal(r.config, otherRustformation.config)
}

func (r *rustformationIR) Validate() error {
	if r == nil || r.config == nil {
		return nil
	}
	return r.config.ValidateAll()
}

// constructRustformation constructs the rustformation policy IR from the policy specification.
func constructRustformation(in *kgateway.TrafficPolicy, out *trafficPolicySpecIr) error {
	if in.Spec.Transformation == nil {
		return nil
	}
	rustformation, err := toRustFormationPerRouteConfig(in.Spec.Transformation)
	if err != nil {
		return err
	}
	out.rustformation = &rustformationIR{
		config: rustformation,
	}
	return nil
}

// toRustFormationPerRouteConfig converts a TransformationPolicy to a RustFormation per route config.
// The shape of this function currently resembles that of the traditional API
// Feel free to change the shape and flow of this function as needed provided there are sufficient unit tests on the configuration output.
// The most dangerous updates here will be any switch over env variables that we are working on.s
func toRustFormationPerRouteConfig(t *kgateway.TransformationPolicy) (*dynamicmodulesv3.DynamicModuleFilterPerRoute, error) {
	if t == nil || *t == (kgateway.TransformationPolicy{}) {
		return nil, nil
	}
	rustformationJson, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}

	stringConf := string(rustformationJson)
	filterCfg, err := utils.MessageToAny(&wrapperspb.StringValue{
		Value: stringConf,
	})
	if err != nil {
		return nil, err
	}
	rustCfg := &dynamicmodulesv3.DynamicModuleFilterPerRoute{
		DynamicModuleConfig: &extensiondynamicmodulev3.DynamicModuleConfig{
			Name: "rust_module",
		},
		PerRouteConfigName: "rustformation",
		FilterConfig:       filterCfg,
	}

	return rustCfg, nil
}

func (p *trafficPolicyPluginGwPass) handleRustFormation(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, rustTransform *rustformationIR) {
	if rustTransform == nil {
		return
	}
	if rustTransform.config != nil {
		typedFilterConfig.AddTypedConfig(rustformationFilterNamePrefix, rustTransform.config)
		p.setTransformationInChain[fcn] = true
	}
}
