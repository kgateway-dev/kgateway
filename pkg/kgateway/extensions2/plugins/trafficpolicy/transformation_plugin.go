package trafficpolicy

import (
	"encoding/json"
	"fmt"
	"slices"

	extensiondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	RustformationModuleName = "rust_module"
	RustformationFilterName = "rustformation"
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
func constructRustformation(
	krtctx krt.HandlerContext,
	in *kgateway.TrafficPolicy,
	commoncol *collections.CommonCollections,
	out *trafficPolicySpecIr,
) error {
	if in.Spec.Transformation == nil {
		return nil
	}

	secrets, err := resolveTransformationSecrets(krtctx, in, commoncol)
	if err != nil {
		return err
	}

	rustformation, err := toRustFormationPerRouteConfig(in.Spec.Transformation, secrets)
	if err != nil {
		return err
	}
	out.rustformation = &rustformationIR{
		config: rustformation,
	}
	return nil
}

func resolveTransformationSecrets(krtctx krt.HandlerContext, policy *kgateway.TrafficPolicy, commoncol *collections.CommonCollections) (map[string]map[string]string, error) {
	resolved := make(map[string]map[string]string)

	from := krtcollections.From{
		GroupKind: wellknown.TrafficPolicyGVK.GroupKind(),
		Namespace: policy.Namespace,
	}

	var secretRefs []kgateway.TransformationSecretRef
	if policy.Spec.Transformation.Request != nil {
		secretRefs = append(secretRefs, policy.Spec.Transformation.Request.SecretRefs...)
	}
	if policy.Spec.Transformation.Response != nil {
		secretRefs = append(secretRefs, policy.Spec.Transformation.Response.SecretRefs...)
	}

	for _, ref := range secretRefs {
		if _, exists := resolved[ref.Name]; exists {
			continue // Already resolved
		}

		secret, err := commoncol.Secrets.GetSecret(krtctx, from, ref.Secret)
		if err != nil {
			return nil, fmt.Errorf("failed to get secret %s for transformation: %w", ref.Name, err)
		}

		data := make(map[string]string)
		for k, v := range secret.Data {
			data[k] = string(v)
		}
		resolved[ref.Name] = data
	}
	return resolved, nil
}

type rustTransformationPolicy struct {
	Request  *kgateway.Transform          `json:"request,omitempty"`
	Response *kgateway.Transform          `json:"response,omitempty"`
	Secrets  map[string]map[string]string `json:"secrets,omitempty"`
}

// toRustFormationPerRouteConfig converts a TransformationPolicy to a RustFormation per route config.
// The shape of this function currently resembles that of the traditional API
// Feel free to change the shape and flow of this function as needed provided there are sufficient unit tests on the configuration output.
// The most dangerous updates here will be any switch over env variables that we are working on.s
func toRustFormationPerRouteConfig(t *kgateway.TransformationPolicy, secrets map[string]map[string]string) (*dynamicmodulesv3.DynamicModuleFilterPerRoute, error) {
	if t == nil || *t == (kgateway.TransformationPolicy{}) {
		return nil, nil
	}
	rustformationJson, err := json.Marshal(&rustTransformationPolicy{
		Request:  t.Request,
		Response: t.Response,
		Secrets:  secrets,
	})
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
			Name: RustformationModuleName,
		},
		FilterName:         RustformationFilterName,
		PerRouteConfigName: RustformationFilterName,
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

func GenerateBlankTransformationConfig() *dynamicmodulesv3.DynamicModuleFilter {
	return &dynamicmodulesv3.DynamicModuleFilter{
		DynamicModuleConfig: &extensiondynamicmodulev3.DynamicModuleConfig{
			Name: RustformationModuleName,
		},
		FilterName: RustformationFilterName,
		FilterConfig: utils.MustMessageToAny(&wrapperspb.StringValue{
			Value: "{}",
		}),
	}
}

func GenerateBlankTransformationConfigPerRoute() *dynamicmodulesv3.DynamicModuleFilterPerRoute {
	return &dynamicmodulesv3.DynamicModuleFilterPerRoute{
		DynamicModuleConfig: &extensiondynamicmodulev3.DynamicModuleConfig{
			Name: RustformationModuleName,
		},
		FilterName:         RustformationFilterName,
		PerRouteConfigName: RustformationFilterName,
		FilterConfig: utils.MustMessageToAny(&wrapperspb.StringValue{
			Value: "{}",
		}),
	}
}

func generateDynamicMetadata(ns string, kv map[string]kgateway.InjaTemplate) *dynamicmodulesv3.DynamicModuleFilterPerRoute {
	var metadata []kgateway.DynamicMetadataTransformation

	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		v := kv[k]
		metadata = append(metadata, kgateway.DynamicMetadataTransformation{
			Namespace: ns,
			Key:       k,
			Value: kgateway.DynamicMetadataValue{
				StringValue: &v,
			},
		})
	}
	b, _ := json.Marshal(&kgateway.TransformationPolicy{
		Request: &kgateway.Transform{
			DynamicMetadata: metadata,
			Body: &kgateway.BodyTransformation{
				ParseAs: kgateway.BodyParseBehaviorNone,
			},
		},
		Response: &kgateway.Transform{
			Body: &kgateway.BodyTransformation{
				ParseAs: kgateway.BodyParseBehaviorNone,
			},
		},
	})
	return &dynamicmodulesv3.DynamicModuleFilterPerRoute{
		DynamicModuleConfig: &extensiondynamicmodulev3.DynamicModuleConfig{
			Name: RustformationModuleName,
		},
		FilterName:         RustformationFilterName,
		PerRouteConfigName: RustformationFilterName,
		FilterConfig: utils.MustMessageToAny(&wrapperspb.StringValue{
			Value: string(b),
		}),
	}
}
