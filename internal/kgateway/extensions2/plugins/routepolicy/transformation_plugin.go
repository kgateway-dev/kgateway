package routepolicy

import (
	"encoding/json"

	// cncfcorev3 "github.com/cncf/xds/go/xds/core/v3"
	// cncftypev3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	// v31 "github.com/cncf/xds/go/xds/type/matcher/v3"
	// corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	// extensionmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/matching/v3"
	exteniondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

func toTraditionalTransform(t *v1alpha1.Transform) *transformationpb.Transformation_TransformationTemplate {
	if t == nil {
		return nil
	}
	hasTransform := false
	tt := &transformationpb.Transformation_TransformationTemplate{
		TransformationTemplate: &transformationpb.TransformationTemplate{
			Headers: map[string]*transformationpb.InjaTemplate{},
		},
	}
	for _, h := range t.Set {
		tt.TransformationTemplate.GetHeaders()[string(h.Name)] = &transformationpb.InjaTemplate{
			Text: string(h.Value),
		}
		tt.TransformationTemplate.ParseBodyBehavior = transformationpb.TransformationTemplate_DontParse
		hasTransform = true
	}

	for _, h := range t.Add {
		tt.TransformationTemplate.HeadersToAppend = append(tt.TransformationTemplate.GetHeadersToAppend(), &transformationpb.TransformationTemplate_HeaderToAppend{
			Key: string(h.Name),
			Value: &transformationpb.InjaTemplate{
				Text: string(h.Value),
			},
		})
		tt.TransformationTemplate.ParseBodyBehavior = transformationpb.TransformationTemplate_DontParse
		hasTransform = true
	}

	tt.TransformationTemplate.HeadersToRemove = make([]string, 0, len(t.Remove))
	for _, h := range t.Remove {
		tt.TransformationTemplate.HeadersToRemove = append(tt.TransformationTemplate.GetHeadersToRemove(), string(h))
		hasTransform = true
	}

	//BODY
	if t.Body == nil {
		tt.TransformationTemplate.BodyTransformation = &transformationpb.TransformationTemplate_Passthrough{
			Passthrough: &transformationpb.Passthrough{},
		}
	} else {
		if t.Body.ParseAs == v1alpha1.BodyParseBehaviorAsString {
			tt.TransformationTemplate.ParseBodyBehavior = transformationpb.TransformationTemplate_DontParse
		}
		if value := t.Body.Value; value != nil {
			hasTransform = true
			tt.TransformationTemplate.BodyTransformation = &transformationpb.TransformationTemplate_Body{
				Body: &transformationpb.InjaTemplate{
					Text: string(*value),
				},
			}
		}
	}

	if !hasTransform {
		return nil
	}
	return tt
}

func toTransformFilterConfig(t *v1alpha1.TransformationPolicy) (*anypb.Any, error) {
	if t == nil || *t == (v1alpha1.TransformationPolicy{}) {
		return nil, nil
	}

	var reqt *transformationpb.Transformation
	var respt *transformationpb.Transformation

	if rtt := toTraditionalTransform(t.Request); rtt != nil {
		reqt = &transformationpb.Transformation{
			TransformationType: rtt,
		}
	}
	if rtt := toTraditionalTransform(t.Response); rtt != nil {
		respt = &transformationpb.Transformation{
			TransformationType: rtt,
		}
	}
	if reqt == nil && respt == nil {
		return nil, nil
	}

	reqm := &transformationpb.RouteTransformations_RouteTransformation_RequestMatch{
		RequestTransformation:  reqt,
		ResponseTransformation: respt,
	}

	envoyT := &transformationpb.RouteTransformations{
		Transformations: []*transformationpb.RouteTransformations_RouteTransformation{
			{

				Match: &transformationpb.RouteTransformations_RouteTransformation_RequestMatch_{
					RequestMatch: reqm,
				},
			},
		},
	}
	return utils.MessageToAny(envoyT)
}

func toRustFormationPerRouteConfig(t *v1alpha1.Transform) (map[string]interface{}, bool) {
	// if there is no transformations present then return a
	hasTransform := false
	rustformationConfigMap := map[string]interface{}{}
	if t == nil {
		return rustformationConfigMap, hasTransform
	}

	// we dont currently have strongly typed objects in rustformation
	setter := make([][2]string, 0, len(t.Set)/2)
	for _, h := range t.Set {
		setter = append(setter, [2]string{string(h.Name), string(h.Value)})
	}

	rustformationConfigMap["headers_setter"] = setter
	if len(setter) > 0 {
		hasTransform = true
	}

	//BODY
	// if t.Body == nil {
	// 	tt.TransformationTemplate.BodyTransformation = &transformationpb.TransformationTemplate_Passthrough{
	// 		Passthrough: &transformationpb.Passthrough{},
	// 	}
	// } else {
	// 	if t.Body.ParseAs == v1alpha1.BodyParseBehaviorAsString {
	// 		tt.TransformationTemplate.ParseBodyBehavior = transformationpb.TransformationTemplate_DontParse
	// 	}
	// 	if value := t.Body.Value; value != nil {
	// 		hasTransform = true
	// 		tt.TransformationTemplate.BodyTransformation = &transformationpb.TransformationTemplate_Body{
	// 			Body: &transformationpb.InjaTemplate{
	// 				Text: string(*value),
	// 			},
	// 		}
	// 	}
	// }
	return rustformationConfigMap, hasTransform
}

// torustformFilterConfig converts a TransformationPolicy to a RustFormation filter config.
// The sheape of this function currently resembles that of the traditional API
// Feel free to change the shape and flow of this function as needed provided there are sufficient unit tests on the configuration output.
// The most dangerous updates here will be any switch over env variables that we are working on.s
func torustformFilterConfig(t *v1alpha1.TransformationPolicy) (*anypb.Any, string, error) {
	if t == nil || *t == (v1alpha1.TransformationPolicy{}) {
		return nil, "", nil
	}
	hasTransform := false
	rustformCfgMap := map[string]interface{}{}

	requestMap, hasRequestTransform := toRustFormationPerRouteConfig(t.Request)
	hasTransform = hasTransform || hasRequestTransform
	for k, v := range requestMap {
		rustformCfgMap["request_"+k] = v
	}

	requestMap, hasResponseTransform := toRustFormationPerRouteConfig(t.Response)
	hasTransform = hasTransform || hasResponseTransform
	for k, v := range requestMap {
		rustformCfgMap["response_"+k] = v
	}

	if !hasTransform {
		return nil, "", nil
	}

	rustformationJson, err := json.Marshal(rustformCfgMap)
	if err != nil {
		return nil, "", err
	}

	stringConf := string(rustformationJson)
	rustCfg := dynamicmodulesv3.DynamicModuleFilter{
		DynamicModuleConfig: &exteniondynamicmodulev3.DynamicModuleConfig{
			Name: "rust_module",
		},
		FilterName:   "http_simple_mutations",
		FilterConfig: stringConf,
	}
	rustCfgAny, _ := utils.MessageToAny(&rustCfg)

	return rustCfgAny, stringConf, nil
}
