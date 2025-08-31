package plugins

import (
	"fmt"
	"log/slog"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/utils"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	extauthPolicySuffix = ":extauth"
	aiPolicySuffix      = ":ai"
	rbacPolicySuffix    = ":rbac"
)

var logger = logging.New("agentgateway/plugins")

// NewTrafficPlugin creates a new TrafficPolicy plugin
func NewTrafficPlugin(agw *AgwCollections) AgentgatewayPlugin {
	col := krt.WrapClient(kclient.NewFiltered[*v1alpha1.TrafficPolicy](
		agw.Client,
		kclient.Filter{ObjectFilter: agw.Client.ObjectFilter()},
	), agw.KrtOpts.ToOptions("TrafficPolicy")...)
	policyCol := krt.NewManyCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.TrafficPolicy) []ADPPolicy {
		return TranslateTrafficPolicy(krtctx, agw.GatewayExtensions, agw.Backends, policyCR)
	})

	return AgentgatewayPlugin{
		ContributesPolicies: map[schema.GroupKind]PolicyPlugin{
			wellknown.TrafficPolicyGVK.GroupKind(): {
				Policies: policyCol,
			},
		},
		ExtraHasSynced: func() bool {
			return policyCol.HasSynced()
		},
	}
}

// TranslateTrafficPolicy generates policies for a single traffic policy
func TranslateTrafficPolicy(
	ctx krt.HandlerContext,
	gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension],
	backends krt.Collection[*v1alpha1.Backend],
	trafficPolicy *v1alpha1.TrafficPolicy,
) []ADPPolicy {
	logger := logger.With("plugin_kind", "traffic")
	var adpPolicies []ADPPolicy

	isMcpTarget := false
	for _, target := range trafficPolicy.Spec.TargetRefs {
		var policyTarget *api.PolicyTarget

		switch string(target.Kind) {
		case wellknown.GatewayKind:
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Gateway{
					Gateway: utils.InternalGatewayName(trafficPolicy.Namespace, string(target.Name), ""),
				},
			}
			if target.SectionName != nil {
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_Listener{
						Listener: utils.InternalGatewayName(trafficPolicy.Namespace, string(target.Name), string(*target.SectionName)),
					},
				}
			}

		case wellknown.HTTPRouteKind:
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: utils.InternalRouteRuleName(trafficPolicy.Namespace, string(target.Name), ""),
				},
			}
			if target.SectionName != nil {
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_RouteRule{
						RouteRule: utils.InternalRouteRuleName(trafficPolicy.Namespace, string(target.Name), string(*target.SectionName)),
					},
				}
			}

		case wellknown.BackendGVK.Kind:
			// kgateway backend kind (MCP, AI, etc.)

			// Look up the Backend referenced by the policy
			backendKey := getBackendKey(trafficPolicy.Namespace, string(target.Name))
			backend := krt.FetchOne(ctx, backends, krt.FilterKey(backendKey))
			if backend == nil {
				logger.Error("backend not found",
					"target", target.Name,
					"policy", client.ObjectKeyFromObject(trafficPolicy))
				return nil
			}
			backendSpec := (*backend).Spec
			if backendSpec.Type == v1alpha1.BackendTypeMCP {
				isMcpTarget = true
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_Backend{
						Backend: trafficPolicy.Namespace + "/" + string(target.Name),
					},
				}
			} else {
				logger.Warn("unsupported target kind. only MCP backends are supported",
					"kind", target.Kind,
					"policy", client.ObjectKeyFromObject(trafficPolicy))
				continue
			}
		default:
			// TODO(npolshak): support attaching policies to k8s services, serviceentries, and other backends
			logger.Warn("unsupported target kind", "kind", target.Kind, "policy", trafficPolicy.Name)
			continue
		}

		if policyTarget != nil {
			translatedPolicies := translateTrafficPolicyToADP(ctx, gatewayExtensions, trafficPolicy, string(target.Name), policyTarget, isMcpTarget)
			adpPolicies = append(adpPolicies, translatedPolicies...)
		}
	}

	return adpPolicies
}

// translateTrafficPolicyToADP converts a TrafficPolicy to agentgateway Policy resources
func translateTrafficPolicyToADP(
	ctx krt.HandlerContext,
	gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension],
	trafficPolicy *v1alpha1.TrafficPolicy,
	policyTargetName string,
	policyTarget *api.PolicyTarget,
	isMcpTarget bool,
) []ADPPolicy {
	adpPolicies := make([]ADPPolicy, 0)

	// Generate a base policy name from the TrafficPolicy reference
	policyName := getTrafficPolicyName(trafficPolicy.Namespace, trafficPolicy.Name, policyTargetName)

	// Convert ExtAuth policy if present
	if trafficPolicy.Spec.ExtAuth != nil && trafficPolicy.Spec.ExtAuth.ExtensionRef != nil {
		extAuthPolicies := processExtAuthPolicy(ctx, gatewayExtensions, trafficPolicy, policyName, policyTarget)
		adpPolicies = append(adpPolicies, extAuthPolicies...)
	}

	// Conver RBAC policy if present
	if trafficPolicy.Spec.RBAC != nil {
		rbacPolicies := processRBACPolicy(trafficPolicy, policyName, policyTarget, isMcpTarget)
		adpPolicies = append(adpPolicies, rbacPolicies...)
	}

	// Process AI policies if present
	if trafficPolicy.Spec.AI != nil {
		aiPolicies := processAIPolicy(ctx, trafficPolicy, policyName, policyTarget)
		adpPolicies = append(adpPolicies, aiPolicies...)
	}

	return adpPolicies
}

// processExtAuthPolicy processes ExtAuth configuration and creates corresponding agentgateway policies
func processExtAuthPolicy(ctx krt.HandlerContext, gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension], trafficPolicy *v1alpha1.TrafficPolicy, policyName string, policyTarget *api.PolicyTarget) []ADPPolicy {
	// Look up the GatewayExtension referenced by the ExtAuth policy
	extensionName := trafficPolicy.Spec.ExtAuth.ExtensionRef.Name
	extensionNamespace := string(ptr.Deref(trafficPolicy.Spec.ExtAuth.ExtensionRef.Namespace, ""))
	if extensionNamespace == "" {
		extensionNamespace = trafficPolicy.Namespace
	}
	gwExtKey := getGatewayExtensionKey(extensionNamespace, string(extensionName))
	gwExt := krt.FetchOne(ctx, gatewayExtensions, krt.FilterKey(gwExtKey))

	if gwExt == nil || (*gwExt).Spec.Type != v1alpha1.GatewayExtensionTypeExtAuth || (*gwExt).Spec.ExtAuth == nil {
		logger.Error("gateway extension not found or not of type ExtAuth", "extension", gwExtKey)
		return nil
	}
	extAuth := (*gwExt).Spec.ExtAuth

	// Extract service target from GatewayExtension's ExtAuth configuration
	var extauthSvcTarget *api.BackendReference
	if extAuth.GrpcService != nil && extAuth.GrpcService.BackendRef != nil {
		backendRef := extAuth.GrpcService.BackendRef
		serviceName := string(backendRef.Name)
		port := uint32(80) // default port
		if backendRef.Port != nil {
			port = uint32(*backendRef.Port)
		}
		// use trafficPolicy namespace as default
		namespace := trafficPolicy.Namespace
		if backendRef.Namespace != nil {
			namespace = string(*backendRef.Namespace)
		}
		serviceHost := kubeutils.ServiceFQDN(metav1.ObjectMeta{Namespace: namespace, Name: serviceName})
		extauthSvcTarget = &api.BackendReference{
			Kind: &api.BackendReference_Service{Service: namespace + "/" + serviceHost},
			Port: port,
		}
	}

	if extauthSvcTarget == nil {
		logger.Warn("failed to translate traffic policy", "policy", trafficPolicy.Name, "target", policyTarget, "error", "missing extauthservice target")
		return nil
	}

	extauthPolicy := &api.Policy{
		Name:   policyName + extauthPolicySuffix,
		Target: policyTarget,
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_ExtAuthz{
				ExtAuthz: &api.PolicySpec_ExternalAuth{
					Target:  extauthSvcTarget,
					Context: trafficPolicy.Spec.ExtAuth.ContextExtensions,
				},
			},
		},
	}

	logger.Debug("generated ExtAuth policy",
		"policy", trafficPolicy.Name,
		"agentgateway_policy", extauthPolicy.Name,
		"target", extauthSvcTarget)

	return []ADPPolicy{{Policy: extauthPolicy}}
}

// processAIPolicy processes AI configuration and creates corresponding ADP policies
func processAIPolicy(ctx krt.HandlerContext, trafficPolicy *v1alpha1.TrafficPolicy, policyName string, policyTarget *api.PolicyTarget) []ADPPolicy {
	logger := logging.New("agentgateway/plugins/traffic")

	aiSpec := trafficPolicy.Spec.AI

	aiPolicy := &api.Policy{
		Name:   policyName + aiPolicySuffix,
		Target: policyTarget,
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_Ai_{
				Ai: &api.PolicySpec_Ai{},
			},
		},
	}

	if aiSpec.PromptEnrichment != nil {
		aiPolicy.GetSpec().GetAi().Prompts = processPromptEnrichment(aiSpec.PromptEnrichment)
	}

	if len(aiSpec.Defaults) > 0 {
		for _, def := range aiSpec.Defaults {
			if def.Override != nil && *def.Override {
				if aiPolicy.GetSpec().GetAi().Overrides == nil {
					aiPolicy.GetSpec().GetAi().Overrides = make(map[string]string)
				}
				aiPolicy.GetSpec().GetAi().Overrides[def.Field] = def.Value
			} else {
				if aiPolicy.GetSpec().GetAi().Defaults == nil {
					aiPolicy.GetSpec().GetAi().Defaults = make(map[string]string)
				}
				aiPolicy.GetSpec().GetAi().Defaults[def.Field] = def.Value
			}
		}
	}

	if aiSpec.PromptGuard != nil {
		if aiPolicy.GetSpec().GetAi().PromptGuard == nil {
			aiPolicy.GetSpec().GetAi().PromptGuard = &api.PolicySpec_Ai_PromptGuard{}
		}
		if aiSpec.PromptGuard.Request != nil {
			aiPolicy.GetSpec().GetAi().PromptGuard.Request = processRequestGuard(aiSpec.PromptGuard.Request, logger)
		}

		if aiSpec.PromptGuard.Response != nil {
			aiPolicy.GetSpec().GetAi().PromptGuard.Response = processResponseGuard(aiSpec.PromptGuard.Response, logger)
		}
	}

	logger.Debug("generated AI policy",
		"policy", trafficPolicy.Name,
		"agentgateway_policy", aiPolicy.Name)

	return []ADPPolicy{{Policy: aiPolicy}}
}

func processRequestGuard(req *v1alpha1.PromptguardRequest, logger *slog.Logger) *api.PolicySpec_Ai_RequestGuard {
	if req == nil {
		return nil
	}

	pgReq := &api.PolicySpec_Ai_RequestGuard{}

	if req.CustomResponse != nil {
		pgReq.Rejection = &api.PolicySpec_Ai_RequestRejection{
			Body:   []byte(*req.CustomResponse.Message),
			Status: *req.CustomResponse.StatusCode,
		}
	}

	if req.Webhook != nil {
		pgReq.Webhook = processWebhook(req.Webhook)
	}

	if req.Regex != nil {
		pgReq.Regex = processRegex(req.Regex, req.CustomResponse, logger)
	}

	if req.Moderation != nil {
		pgReq.OpenaiModeration = processModeration(req.Moderation)
	}

	return pgReq
}

func processResponseGuard(resp *v1alpha1.PromptguardResponse, logger *slog.Logger) *api.PolicySpec_Ai_ResponseGuard {
	pgResp := &api.PolicySpec_Ai_ResponseGuard{}

	if resp.Webhook != nil {
		pgResp.Webhook = processWebhook(resp.Webhook)
	}

	if resp.Regex != nil {
		pgResp.Regex = processRegex(resp.Regex, nil, logger)
	}

	return pgResp
}

func processPromptEnrichment(enrichment *v1alpha1.AIPromptEnrichment) *api.PolicySpec_Ai_PromptEnrichment {
	pgPromptEnrichment := &api.PolicySpec_Ai_PromptEnrichment{}

	// Add prepend messages
	for _, msg := range enrichment.Prepend {
		pgPromptEnrichment.Prepend = append(pgPromptEnrichment.Prepend, &api.PolicySpec_Ai_Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Add append messages
	for _, msg := range enrichment.Append {
		pgPromptEnrichment.Append = append(pgPromptEnrichment.Append, &api.PolicySpec_Ai_Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return pgPromptEnrichment
}

func processWebhook(webhook *v1alpha1.Webhook) *api.PolicySpec_Ai_Webhook {
	if webhook == nil {
		return nil
	}
	return &api.PolicySpec_Ai_Webhook{
		Host: webhook.Host.Host,
		Port: uint32(webhook.Host.Port),
	}
}

func processBuiltinRegexRule(builtin v1alpha1.BuiltIn, logger *slog.Logger) *api.PolicySpec_Ai_RegexRule {
	builtinValue, ok := api.PolicySpec_Ai_BuiltinRegexRule_value[string(builtin)]
	if !ok {
		logger.Warn("unknown builtin regex rule", "builtin", builtin)
		builtinValue = int32(api.PolicySpec_Ai_BUILTIN_UNSPECIFIED)
	}
	return &api.PolicySpec_Ai_RegexRule{
		Kind: &api.PolicySpec_Ai_RegexRule_Builtin{
			Builtin: api.PolicySpec_Ai_BuiltinRegexRule(builtinValue),
		},
	}
}

func processNamedRegexRule(pattern, name string) *api.PolicySpec_Ai_RegexRule {
	return &api.PolicySpec_Ai_RegexRule{
		Kind: &api.PolicySpec_Ai_RegexRule_Regex{
			Regex: &api.PolicySpec_Ai_NamedRegex{
				Pattern: pattern,
				Name:    name,
			},
		},
	}
}

func processRegex(regex *v1alpha1.Regex, customResponse *v1alpha1.CustomResponse, logger *slog.Logger) *api.PolicySpec_Ai_RegexRules {
	if regex == nil {
		return nil
	}

	rules := &api.PolicySpec_Ai_RegexRules{}
	if regex.Action != nil {
		rules.Action = &api.PolicySpec_Ai_Action{}
		if *regex.Action == v1alpha1.MASK {
			rules.Action.Kind = api.PolicySpec_Ai_MASK
		} else if *regex.Action == v1alpha1.REJECT {
			rules.Action.Kind = api.PolicySpec_Ai_REJECT
			rules.Action.RejectResponse = &api.PolicySpec_Ai_RequestRejection{}
			if customResponse != nil {
				if customResponse.Message != nil {
					rules.Action.RejectResponse.Body = []byte(*customResponse.Message)
				}
				if customResponse.StatusCode != nil {
					rules.Action.RejectResponse.Status = *customResponse.StatusCode
				}
			}
		} else {
			logger.Warn("unsupported regex action", "action", *regex.Action)
			rules.Action.Kind = api.PolicySpec_Ai_ACTION_UNSPECIFIED
		}
	}

	for _, match := range regex.Matches {
		// TODO(jmcguire98): should we really allow empty patterns on regex matches?
		// I see the CRD is omitempty, but I don't get why
		// for now i'm just dropping them on the floor
		if match.Pattern == nil {
			continue
		}

		// we should probably not pass an empty name to the dataplane even if none was provided,
		// since the name is what will be used for masking
		// if the action is mask
		name := ""
		if match.Name != nil {
			name = *match.Name
		}

		rules.Rules = append(rules.Rules, processNamedRegexRule(*match.Pattern, name))
	}

	for _, builtin := range regex.Builtins {
		rules.Rules = append(rules.Rules, processBuiltinRegexRule(builtin, logger))
	}

	return rules
}

func processModeration(moderation *v1alpha1.Moderation) *api.PolicySpec_Ai_Moderation {
	// right now we only support OpenAI moderation, so we can return nil if the moderation is nil or the OpenAIModeration is nil
	if moderation == nil || moderation.OpenAIModeration == nil {
		return nil
	}

	pgModeration := &api.PolicySpec_Ai_Moderation{}

	if moderation.OpenAIModeration.Model != nil {
		pgModeration.Model = &wrapperspb.StringValue{
			Value: *moderation.OpenAIModeration.Model,
		}
	}

	switch moderation.OpenAIModeration.AuthToken.Kind {
	case v1alpha1.Inline:
		if moderation.OpenAIModeration.AuthToken.Inline != nil {
			pgModeration.Auth = &api.BackendAuthPolicy{
				Kind: &api.BackendAuthPolicy_Key{
					Key: &api.Key{
						Secret: *moderation.OpenAIModeration.AuthToken.Inline,
					},
				},
			}
		}
	case v1alpha1.SecretRef:
		if moderation.OpenAIModeration.AuthToken.SecretRef != nil {
			pgModeration.Auth = &api.BackendAuthPolicy{
				Kind: &api.BackendAuthPolicy_Key{
					Key: &api.Key{
						Secret: moderation.OpenAIModeration.AuthToken.SecretRef.Name,
					},
				},
			}
		}
	case v1alpha1.Passthrough:
		pgModeration.Auth = &api.BackendAuthPolicy{
			Kind: &api.BackendAuthPolicy_Passthrough{
				Passthrough: &api.Passthrough{},
			},
		}
	}

	return pgModeration
}

// processRBACPolicy processes RBAC configuration and creates corresponding ADP policies
func processRBACPolicy(
	trafficPolicy *v1alpha1.TrafficPolicy,
	policyName string,
	policyTarget *api.PolicyTarget,
	isMCP bool,
) []ADPPolicy {
	logger := logging.New("agentgateway/plugins/traffic/rbac")

	var allowPolicies, denyPolicies []string
	if trafficPolicy.Spec.RBAC.Action == v1alpha1.AuthorizationPolicyActionDeny {
		denyPolicies = append(denyPolicies, trafficPolicy.Spec.RBAC.Policy.MatchExpressions...)
	} else {
		allowPolicies = append(allowPolicies, trafficPolicy.Spec.RBAC.Policy.MatchExpressions...)
	}

	var rbacPolicy *api.Policy
	if isMCP {
		rbacPolicy = &api.Policy{
			Name:   policyName + rbacPolicySuffix,
			Target: policyTarget,
			Spec: &api.PolicySpec{
				Kind: &api.PolicySpec_McpAuthorization{
					McpAuthorization: &api.PolicySpec_RBAC{
						Allow: allowPolicies,
						Deny:  denyPolicies,
					},
				},
			},
		}
	} else {
		rbacPolicy = &api.Policy{
			Name:   policyName + rbacPolicySuffix,
			Target: policyTarget,
			Spec: &api.PolicySpec{
				Kind: &api.PolicySpec_Authorization{
					Authorization: &api.PolicySpec_RBAC{
						Allow: allowPolicies,
						Deny:  denyPolicies,
					},
				},
			},
		}
	}

	logger.Debug("generated RBAC policy",
		"policy", trafficPolicy.Name,
		"agentgateway_policy", rbacPolicy.Name,
		"target", policyTarget)

	return []ADPPolicy{{Policy: rbacPolicy}}
}

func getTrafficPolicyName(trafficPolicyNs, trafficPolicyName, policyTargetName string) string {
	return fmt.Sprintf("trafficpolicy/%s/%s/%s", trafficPolicyNs, trafficPolicyName, policyTargetName)
}

func getBackendKey(targetPolicyNs, targetName string) string {
	return fmt.Sprintf("%s/%s", targetPolicyNs, targetName)
}

func getGatewayExtensionKey(extensionNamespace, extensionName string) string {
	return fmt.Sprintf("%s/%s", extensionNamespace, extensionName)
}
