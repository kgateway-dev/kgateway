package trafficpolicy

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"os"
	"reflect"
	"strings"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"github.com/mitchellh/hashstructure"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

const (
	contextString = `{"content":"%s","role":"%s"}`

	// AiDebugTransformations Controls the debugging log behavior of the AI backend's Envoy transformation filter.
	// When this variable is enabled, Envoy will record detailed HTTP request/response information processed by the AI Gateway.
	// This is very helpful for understanding data flow, debugging transformation rules.
	// Expected values: "true" to enable, any other value (or unset) to disable.
	AiDebugTransformations = "AI_PLUGIN_DEBUG_TRANSFORMATIONS"

	// AiListenAddr can be used to test the ext-proc filter locally.
	// Expected values: A valid network address string (e.g., "127.0.0.1:9000").
	AiListenAddr = "AI_PLUGIN_LISTEN_ADDR"
)

// AIPolicyIR is the internal representation of an AI policy.
type aiPolicyIR struct {
	AISecret *ir.Secret
	// Extproc config can come from the AI backend and AI policy
	Extproc *envoy_ext_proc_v3.ExtProcPerRoute
	// Transformations coming from the AI policy
	Transformation *envoytransformation.RouteTransformations
}

var _ PolicySubIR = &aiPolicyIR{}

// Equals checks if two aiPolicyIR instances are equal.
func (a *aiPolicyIR) Equals(in PolicySubIR) bool {
	inAI, ok := in.(*aiPolicyIR)
	if !ok {
		return false
	}
	if a == nil && inAI == nil {
		return true
	}
	if a == nil || inAI == nil {
		return false
	}

	// Check AISecret equality
	if a.AISecret != nil && inAI.AISecret != nil {
		if !a.AISecret.Equals(*inAI.AISecret) {
			return false
		}
	} else if (a.AISecret != nil) != (inAI.AISecret != nil) {
		return false
	}
	// Check Extproc equality
	if !proto.Equal(a.Extproc, inAI.Extproc) {
		return false
	}
	// Check Transformation equality
	if !proto.Equal(a.Transformation, inAI.Transformation) {
		return false
	}

	return true
}

// Validate performs PGV-based validation on the AI policy components
func (a *aiPolicyIR) Validate() error {
	if a == nil {
		return nil
	}
	if a.Transformation != nil {
		if err := a.Transformation.Validate(); err != nil {
			return err
		}
	}
	if a.Extproc != nil {
		if err := a.Extproc.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// constructAI constructs the AI policy IR from the policy specification.
func constructAI(
	krtctx krt.HandlerContext,
	policyCR *v1alpha1.TrafficPolicy,
	secrets *krtcollections.SecretIndex,
	out *trafficPolicySpecIr,
) error {
	if policyCR.Spec.AI == nil {
		return nil
	}

	ir := &aiPolicyIR{}
	// Augment with AI secrets as needed
	secret, err := aiSecretForSpec(krtctx, secrets, policyCR)
	if err != nil {
		return fmt.Errorf("ai: %w", err)
	}
	ir.AISecret = secret
	// Preprocess the AI backend
	if err := preProcessAITrafficPolicy(policyCR.Spec.AI, ir); err != nil {
		return fmt.Errorf("ai: %w", err)
	}
	out.ai = ir
	return nil
}

func (p *trafficPolicyPluginGwPass) processAITrafficPolicy(
	configMap *ir.TypedFilterConfigMap,
	inIr *aiPolicyIR,
) {
	if inIr.Transformation != nil {
		configMap.AddTypedConfig(wellknown.AIPolicyTransformationFilterName, inIr.Transformation)
	}

	if inIr.Extproc != nil {
		clonedExtProcFromIR := proto.Clone(inIr.Extproc).(*envoy_ext_proc_v3.ExtProcPerRoute)
		// Envoy merges GrpcInitialMetadata config from the route, but we need to manually merge if Backend has configured extproc already
		extProcProtoFromPCtx := configMap.GetTypedConfig(wellknown.AIExtProcFilterName)
		if extProcProtoFromPCtx != nil {
			// route policy extproc only configures GrpcInitialMetadata
			clonedExtProcFromIR = extProcProtoFromPCtx.(*envoy_ext_proc_v3.ExtProcPerRoute)
			grpcInitMd := clonedExtProcFromIR.GetOverrides().GetGrpcInitialMetadata()
			grpcInitMd = append(grpcInitMd, inIr.Extproc.GetOverrides().GetGrpcInitialMetadata()...)
			clonedExtProcFromIR.GetOverrides().GrpcInitialMetadata = grpcInitMd
		}
		configMap.AddTypedConfig(wellknown.AIExtProcFilterName, clonedExtProcFromIR)
	}
}

func preProcessAITrafficPolicy(
	aiConfig *v1alpha1.AIPolicy,
	ir *aiPolicyIR,
) error {
	// Setup initial transformation template and extproc settings. The extproc is configured by the route policy and backend.
	transformationTemplate := initTransformationTemplate()
	extprocSettings := &envoy_ext_proc_v3.ExtProcPerRoute{
		Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
			Overrides: &envoy_ext_proc_v3.ExtProcOverrides{},
		},
	}

	// If the route options specify this as a chat streaming route, add a header to the ext-proc request
	if aiConfig.RouteType != nil && *aiConfig.RouteType == v1alpha1.CHAT_STREAMING {
		// append streaming header if it's a streaming route
		extprocSettings.GetOverrides().GrpcInitialMetadata = append(extprocSettings.GetOverrides().GetGrpcInitialMetadata(), &envoycorev3.HeaderValue{
			Key:   "x-chat-streaming",
			Value: "true",
		})
		transformationTemplate.DynamicMetadataValues = append(transformationTemplate.GetDynamicMetadataValues(), &envoytransformation.TransformationTemplate_DynamicMetadataValue{
			Key:   "route_type",
			Value: &envoytransformation.InjaTemplate{Text: "CHAT_STREAMING"},
		})
	}

	err := handleAITrafficPolicy(aiConfig, extprocSettings, transformationTemplate, ir.AISecret)
	if err != nil {
		return err
	}

	routeTransformations := &envoytransformation.RouteTransformations{
		Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{
			{
				Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
					RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
						RequestTransformation: &envoytransformation.Transformation{
							// Set this env var to true to log the request/response info for each transformation
							LogRequestResponseInfo: wrapperspb.Bool(os.Getenv(AiDebugTransformations) == "true"),
							TransformationType: &envoytransformation.Transformation_TransformationTemplate{
								TransformationTemplate: transformationTemplate,
							},
						},
					},
				},
			},
		},
	}
	ir.Transformation = routeTransformations
	ir.Extproc = extprocSettings

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

func handleAITrafficPolicy(
	aiConfig *v1alpha1.AIPolicy,
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

		value := strings.TrimSpace(field.Value)

		// Inja template cannot recognize if a JSON string is valid, so we need to pre-validate based on JSON object/array format
		// Valid object: {"model":"gpt4"}
		// Valid array: [1,2,3]
		// Invalid formats: {"model":"gpt4", "model2":}, [1,2,3, [1,2,3
		if hasJsonPrefix(value) || hasJsonSuffix(value) {
			if !json.Valid([]byte(field.Value)) {
				return fmt.Errorf("field %s contains invalid JSON string: %s", field.Field, field.Value)
			}
		}
		// When field.Value is a primitive type, deserialization from byte array works normally, tmpl value is: tmpl = string(marshalled)
		// When field.Value is an object/array, deserialization treats it as a plain string, tmpl value should use the original value: tmpl = field.Value
		var tmpl string
		if field.Override {
			if hasJsonPrefix(value) {
				tmpl = field.Value
			} else {
				tmpl = string(marshalled)
			}
		} else {
			// Inja default function will use the default value if the field provided is falsey
			if hasJsonPrefix(value) {
				tmpl = fmt.Sprintf("{{ default(%s, %s) }}", field.Field, field.Value)
			} else {
				tmpl = fmt.Sprintf("{{ default(%s, %s) }}", field.Field, string(marshalled))
			}
		}
		if transformation.GetMergeJsonKeys().GetJsonKeys() == nil {
			transformation.GetMergeJsonKeys().JsonKeys = make(map[string]*envoytransformation.MergeJsonKeys_OverridableTemplate)
		}
		if bt, ok := transformation.GetBodyTransformation().(*envoytransformation.TransformationTemplate_MergeJsonKeys); ok {
			if bt == nil {
				transformation.BodyTransformation = emptyBodyTransformation()
			}
			transformation.GetMergeJsonKeys().GetJsonKeys()[field.Field] = &envoytransformation.MergeJsonKeys_OverridableTemplate{
				Tmpl: &envoytransformation.InjaTemplate{Text: tmpl},
			}
		}
	}
	return nil
}

// hasJsonPrefix checks if the given JSON string contains object/array opening symbols
func hasJsonPrefix(value string) bool {
	return strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[")
}

// hasJsonSuffix checks if the given JSON string contains object/array closing symbols
func hasJsonSuffix(value string) bool {
	return strings.HasSuffix(value, "}") || strings.HasSuffix(value, "]")
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
	if bt, ok := transformation.GetBodyTransformation().(*envoytransformation.TransformationTemplate_MergeJsonKeys); ok {
		if bt == nil {
			transformation.BodyTransformation = emptyBodyTransformation()
		}
		transformation.GetMergeJsonKeys().GetJsonKeys()["messages"] = &envoytransformation.MergeJsonKeys_OverridableTemplate{
			Tmpl: &envoytransformation.InjaTemplate{Text: finalBody},
		}
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
				token, err := pluginutils.GetAuthToken(mod.OpenAIModeration.AuthToken, secret)
				if err != nil {
					return err
				}
				mod.OpenAIModeration.AuthToken = v1alpha1.SingleAuthToken{
					Kind:   v1alpha1.Inline,
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
			&envoycorev3.HeaderValue{
				Key:   "x-req-guardrails-config",
				Value: string(bin),
			},
		)
		// Use this in the server to key per-route-config
		// Better to do it here because we have generated functions
		reqHash, _ := hashUnique(req, nil)
		extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
			&envoycorev3.HeaderValue{
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
			&envoycorev3.HeaderValue{
				Key:   "x-resp-guardrails-config",
				Value: string(bin),
			},
		)
		// Use this in the server to key per-route-config
		// Better to do it here because we have generated functions
		respHash, _ := hashUnique(resp, nil)
		extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
			&envoycorev3.HeaderValue{
				Key:   "x-resp-guardrails-config-hash",
				Value: fmt.Sprint(respHash),
			},
		)
	}
	return nil
}

// hashUnique generates a hash of the struct that is unique to the object by
// hashing the entire structure using hashstructure.
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

	// Compute hash of the entire struct
	structHash, err := hashstructure.Hash(val.Interface(), &hashstructure.HashOptions{})
	if err != nil {
		return 0, err
	}

	// Write the struct hash to our hasher
	if err := binary.Write(hasher, binary.LittleEndian, structHash); err != nil {
		return 0, err
	}

	return hasher.Sum64(), nil
}

func emptyBodyTransformation() *envoytransformation.TransformationTemplate_MergeJsonKeys {
	return &envoytransformation.TransformationTemplate_MergeJsonKeys{
		MergeJsonKeys: &envoytransformation.MergeJsonKeys{
			JsonKeys: map[string]*envoytransformation.MergeJsonKeys_OverridableTemplate{},
		},
	}
}

// aiSecret checks for the presence of the OpenAI Moderation which may require a secret reference
// will log an error if the secret is needed but not found
func aiSecretForSpec(
	krtctx krt.HandlerContext,
	secrets *krtcollections.SecretIndex,
	policyCR *v1alpha1.TrafficPolicy,
) (*ir.Secret, error) {
	if policyCR.Spec.AI == nil ||
		policyCR.Spec.AI.PromptGuard == nil ||
		policyCR.Spec.AI.PromptGuard.Request == nil ||
		policyCR.Spec.AI.PromptGuard.Request.Moderation == nil ||
		policyCR.Spec.AI.PromptGuard.Request.Moderation.OpenAIModeration == nil {
		return nil, nil
	}

	secretRef := policyCR.Spec.AI.PromptGuard.Request.Moderation.OpenAIModeration.AuthToken.SecretRef
	if secretRef == nil {
		// no secret ref is set
		return nil, nil
	}

	// Retrieve and assign the secret
	secret, err := pluginutils.GetSecretIr(secrets, krtctx, secretRef.Name, policyCR.GetNamespace())
	if err != nil {
		logger.Error("failed to get secret for AI policy", "secret_name", secretRef.Name, "namespace", policyCR.GetNamespace(), "error", err)
		return nil, err
	}
	return secret, nil
}
