package routepolicy

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"os"
	"reflect"
	"strings"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"github.com/mitchellh/hashstructure"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/utils/ptr"

	aiutils "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

const (
	contextString = `{"content":"%s","role":"%s"}`
)

func (p *routePolicyPluginGwPass) processAIRoutePolicy(
	ctx context.Context,
	aiConfig *v1alpha1.AIRoutePolicy,
	pCtx *ir.RouteBackendContext,
	extprocSettings *envoy_ext_proc_v3.ExtProcPerRoute,
	transformations *envoytransformation.RouteTransformations,
	aiSecret *ir.Secret,
) error {
	// If the route options specify this as a chat streaming route, add a header to the ext-proc request
	if aiConfig.RouteType != nil && *aiConfig.RouteType == v1alpha1.CHAT_STREAMING {
		// append streaming header if it's a streaming route
		extprocSettings.GetOverrides().GrpcInitialMetadata = append(extprocSettings.GetOverrides().GetGrpcInitialMetadata(), &envoy_config_core_v3.HeaderValue{
			Key:   "x-chat-streaming",
			Value: "true",
		})
		p.setAIFilter = true
	}

	// Setup initial transformation template. This may be modified by further
	transformationTemplate := initTransformationTemplate()
	err := handleAIRoutePolicy(aiConfig, extprocSettings, transformationTemplate, aiSecret)
	if err != nil {
		return err
	}

	pCtx.AddTypedConfig(wellknown.AIExtProcFilterName, extprocSettings)
	transformation := &envoytransformation.RouteTransformations_RouteTransformation{
		Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
			RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
				RequestTransformation: &envoytransformation.Transformation{
					// Set this env var to true to log the request/response info for each transformation
					LogRequestResponseInfo: wrapperspb.Bool(os.Getenv("AI_PLUGIN_DEBUG_TRANSFORMATIONS") == "true"),
					TransformationType: &envoytransformation.Transformation_TransformationTemplate{
						TransformationTemplate: transformationTemplate,
					},
				},
			},
		},
	}
	// add the disjoint route transformations to the list
	transformations.Transformations = append(transformations.GetTransformations(), transformation)
	pCtx.AddTypedConfig(wellknown.TransformationFilterName, transformations)

	return nil
}

func initTransformationTemplate() *envoytransformation.TransformationTemplate {
	transformationTemplate := &envoytransformation.TransformationTemplate{
		// We will add the auth token later
		Headers: map[string]*envoytransformation.InjaTemplate{},
	}
	transformationTemplate.BodyTransformation = &envoytransformation.TransformationTemplate_MergeJsonKeys{
		MergeJsonKeys: &envoytransformation.MergeJsonKeys{
			JsonKeys: map[string]*envoytransformation.MergeJsonKeys_OverridableTemplate{},
		},
	}
	return transformationTemplate
}

func handleAIRoutePolicy(
	aiConfig *v1alpha1.AIRoutePolicy,
	extProcRouteSettings *envoy_ext_proc_v3.ExtProcPerRoute,
	transformation *envoytransformation.TransformationTemplate,
	aiSecrets *ir.Secret,
) error {
	if err := applyDefaults(aiConfig.Defaults, transformation); err != nil {
		return err
	}

	if err := applyPromptEnrichment(aiConfig.PromptEnrichment, transformation); err != nil {
		return err
	}

	if err := applyPromptGuard(aiConfig.PromptGuard, extProcRouteSettings, aiSecrets); err != nil {
		return err
	}

	return nil
}

func applyDefaults(
	defaults []v1alpha1.FieldDefault,
	transformation *envoytransformation.TransformationTemplate,
) error {
	if len(defaults) == 0 {
		return nil
	}
	for _, field := range defaults {
		marshalled, err := json.Marshal(field.Value)
		if err != nil {
			return err
		}
		var tmpl string
		if field.Override != nil {
			// Inja default function will use the default value if the field provided is falsey
			tmpl = fmt.Sprintf("{{ default(%s, %s) }}", field.Value, string(marshalled))
		} else {
			tmpl = string(marshalled)
		}
		if transformation.GetMergeJsonKeys().GetJsonKeys() == nil {
			transformation.GetMergeJsonKeys().JsonKeys = make(map[string]*envoytransformation.MergeJsonKeys_OverridableTemplate)
		}
		transformation.GetMergeJsonKeys().GetJsonKeys()[field.Field] = &envoytransformation.MergeJsonKeys_OverridableTemplate{
			Tmpl: &envoytransformation.InjaTemplate{Text: tmpl},
		}
	}
	return nil
}

func applyPromptEnrichment(
	pe *v1alpha1.AIPromptEnrichment,
	transformation *envoytransformation.TransformationTemplate,
) error {
	if pe == nil {
		return nil
	}
	// This function does some slightly complex json string work because we're instructing the transformation filter
	// to take the existing `messages` field and potentially prepend and append to it.
	// JSON is insensitive to new lines, so we don't need to worry about them. We simply need to join the
	// user added messages with the request messages
	// For example:
	// messages = [{"content": "welcome ", "role": "user"}]
	// prepend = [{"content": "hi", "role": "user"}]
	// append = [{"content": "bye", "role": "user"}]
	// Would result in:
	// [{"content": "hi", "role": "user"}, {"content": "welcome ", "role": "user"}, {"content": "bye", "role": "user"}]
	bodyChunk1 := `[`
	bodyChunk2 := `{{ join(messages, ", ") }}`
	bodyChunk3 := `]`

	prependString := make([]string, 0, len(pe.Prepend))
	for _, toPrepend := range pe.Prepend {
		prependString = append(
			prependString,
			fmt.Sprintf(
				contextString,
				toPrepend.Content,
				strings.ToLower(strings.ToLower(toPrepend.Role)),
			)+",",
		)
	}
	appendString := make([]string, 0, len(pe.Append))
	for idx, toAppend := range pe.Append {
		formatted := fmt.Sprintf(
			contextString,
			toAppend.Content,
			strings.ToLower(strings.ToLower(toAppend.Role)),
		)
		if idx != len(pe.Append)-1 {
			formatted += ","
		}
		appendString = append(appendString, formatted)
	}
	builder := &strings.Builder{}
	builder.WriteString(bodyChunk1)
	builder.WriteString(strings.Join(prependString, ""))
	builder.WriteString(bodyChunk2)
	if len(appendString) > 0 {
		builder.WriteString(",")
		builder.WriteString(strings.Join(appendString, ""))
	}
	builder.WriteString(bodyChunk3)
	finalBody := builder.String()
	// Overwrite the user messages body key with the templated version
	transformation.GetMergeJsonKeys().GetJsonKeys()["messages"] = &envoytransformation.MergeJsonKeys_OverridableTemplate{
		Tmpl: &envoytransformation.InjaTemplate{Text: finalBody},
	}
	return nil
}

func applyPromptGuard(pg *v1alpha1.AIPromptGuard, extProcRouteSettings *envoy_ext_proc_v3.ExtProcPerRoute, secret *ir.Secret) error {
	if pg == nil {
		return nil
	}
	if req := pg.Request; req != nil {
		if mod := req.Moderation; mod != nil {
			if mod.OpenAIModeration != nil {
				token, err := aiutils.GetAuthToken(mod.OpenAIModeration.AuthToken, secret)
				if err != nil {
					return err
				}
				mod.OpenAIModeration.AuthToken = v1alpha1.SingleAuthToken{
					Inline: ptr.To(token),
				}
			} else {
				return fmt.Errorf("OpenAI moderation config must be set for moderation prompt guard")
			}
			pg.Request.Moderation = mod
		}
		bin, err := json.Marshal(req)
		if err != nil {
			return err
		}
		extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
			&envoy_config_core_v3.HeaderValue{
				Key:   "x-req-guardrails-config",
				Value: string(bin),
			},
		)
		// Use this in the server to key per-route-config
		// Better to do it here because we have generated functions
		reqHash, _ := hashUnique(req, nil)
		extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
			&envoy_config_core_v3.HeaderValue{
				Key:   "x-req-guardrails-config-hash",
				Value: fmt.Sprint(reqHash),
			},
		)
	}

	if resp := pg.Response; resp != nil {
		// Resp needs to be defined in python ai extensions in the same format
		bin, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
			&envoy_config_core_v3.HeaderValue{
				Key:   "x-resp-guardrails-config",
				Value: string(bin),
			},
		)
		// Use this in the server to key per-route-config
		// Better to do it here because we have generated functions
		respHash, _ := hashUnique(resp, nil)
		extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
			&envoy_config_core_v3.HeaderValue{
				Key:   "x-resp-guardrails-config-hash",
				Value: fmt.Sprint(respHash),
			},
		)

	}
	return nil
}

// hashUnique generates a hash of the struct that is unique to the object by
// hashing field name and value pairs
func hashUnique(obj interface{}, hasher hash.Hash64) (uint64, error) {
	if obj == nil {
		return 0, nil
	}
	if hasher == nil {
		hasher = fnv.New64()
	}

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	// Write type name for consistency with proto implementation
	_, err := hasher.Write([]byte(typ.PkgPath() + "/" + typ.Name()))
	if err != nil {
		return 0, err
	}

	// Iterate through fields
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Write field name
		if _, err := hasher.Write([]byte(fieldType.Name)); err != nil {
			return 0, err
		}

		// Handle nil pointer fields
		if field.Kind() == reflect.Ptr && field.IsNil() {
			continue
		}

		// Get the actual value if it's a pointer
		if field.Kind() == reflect.Ptr {
			field = field.Elem()
		}

		// Hash the field value
		fieldValue, err := hashstructure.Hash(field.Interface(), nil)
		if err != nil {
			return 0, err
		}

		// Write the hash to our hasher
		if err := binary.Write(hasher, binary.LittleEndian, fieldValue); err != nil {
			return 0, err
		}
	}

	return hasher.Sum64(), nil
}
