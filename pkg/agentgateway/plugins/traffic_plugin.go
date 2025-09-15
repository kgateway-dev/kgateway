package plugins

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	extauthPolicySuffix         = ":extauth"
	aiPolicySuffix              = ":ai"
	rbacPolicySuffix            = ":rbac"
	localRateLimitPolicySuffix  = ":rl-local"
	globalRateLimitPolicySuffix = ":rl-global"
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
		aiPolicies := processAIPolicy(trafficPolicy, policyName, policyTarget)
		adpPolicies = append(adpPolicies, aiPolicies...)
	}
	// Process RateLimit policies if present
	if trafficPolicy.Spec.RateLimit != nil {
		rateLimitPolicies := processRateLimitPolicy(ctx, gatewayExtensions, trafficPolicy, policyName, policyTarget)
		adpPolicies = append(adpPolicies, rateLimitPolicies...)
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
func processAIPolicy(trafficPolicy *v1alpha1.TrafficPolicy, policyName string, policyTarget *api.PolicyTarget) []ADPPolicy {
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

	for _, def := range aiSpec.Defaults {
		val, err := toJSONValue(def.Value)
		if err != nil {
			logger.Error("error parsing field value", "field", def.Field, "error", err)
			continue
		}
		if def.Override {
			if aiPolicy.GetSpec().GetAi().Overrides == nil {
				aiPolicy.GetSpec().GetAi().Overrides = make(map[string]string)
			}
			aiPolicy.GetSpec().GetAi().Overrides[def.Field] = val
		} else {
			if aiPolicy.GetSpec().GetAi().Defaults == nil {
				aiPolicy.GetSpec().GetAi().Defaults = make(map[string]string)
			}
			aiPolicy.GetSpec().GetAi().Defaults[def.Field] = val
		}
	}

	if aiSpec.PromptGuard != nil {
		if aiPolicy.GetSpec().GetAi().PromptGuard == nil {
			aiPolicy.GetSpec().GetAi().PromptGuard = &api.PolicySpec_Ai_PromptGuard{}
		}
		if aiSpec.PromptGuard.Request != nil {
			aiPolicy.GetSpec().GetAi().PromptGuard.Request = processRequestGuard(aiSpec.PromptGuard.Request)
		}

		if aiSpec.PromptGuard.Response != nil {
			aiPolicy.GetSpec().GetAi().PromptGuard.Response = processResponseGuard(aiSpec.PromptGuard.Response)
		}
	}

	logger.Debug("generated AI policy",
		"policy", trafficPolicy.Name,
		"agentgateway_policy", aiPolicy.Name)

	return []ADPPolicy{{Policy: aiPolicy}}
}

func processRequestGuard(req *v1alpha1.PromptguardRequest) *api.PolicySpec_Ai_RequestGuard {
	if req == nil {
		return nil
	}

	pgReq := &api.PolicySpec_Ai_RequestGuard{
		Webhook:          processWebhook(req.Webhook),
		Regex:            processRegex(req.Regex, req.CustomResponse),
		OpenaiModeration: processModeration(req.Moderation),
	}

	if req.CustomResponse != nil {
		pgReq.Rejection = &api.PolicySpec_Ai_RequestRejection{
			Body:   []byte(*req.CustomResponse.Message),
			Status: *req.CustomResponse.StatusCode,
		}
	}

	return pgReq
}

func processResponseGuard(resp *v1alpha1.PromptguardResponse) *api.PolicySpec_Ai_ResponseGuard {
	return &api.PolicySpec_Ai_ResponseGuard{
		Webhook: processWebhook(resp.Webhook),
		Regex:   processRegex(resp.Regex, nil),
	}
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

	w := &api.PolicySpec_Ai_Webhook{
		Host: webhook.Host.Host,
		Port: uint32(webhook.Host.Port),
	}

	if len(webhook.ForwardHeaderMatches) > 0 {
		headers := make([]*api.HeaderMatch, 0, len(webhook.ForwardHeaderMatches))
		for _, match := range webhook.ForwardHeaderMatches {
			switch ptr.Deref(match.Type, gwv1.HeaderMatchExact) {
			case gwv1.HeaderMatchExact:
				headers = append(headers, &api.HeaderMatch{
					Name:  string(match.Name),
					Value: &api.HeaderMatch_Exact{Exact: match.Value},
				})

			case gwv1.HeaderMatchRegularExpression:
				headers = append(headers, &api.HeaderMatch{
					Name:  string(match.Name),
					Value: &api.HeaderMatch_Regex{Regex: match.Value},
				})
			}
		}
		w.ForwardHeaderMatches = headers
	}

	return w
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

func processRegex(regex *v1alpha1.Regex, customResponse *v1alpha1.CustomResponse) *api.PolicySpec_Ai_RegexRules {
	if regex == nil {
		return nil
	}

	rules := &api.PolicySpec_Ai_RegexRules{}
	if regex.Action != nil {
		rules.Action = &api.PolicySpec_Ai_Action{}
		switch *regex.Action {
		case v1alpha1.MASK:
			rules.Action.Kind = api.PolicySpec_Ai_MASK
		case v1alpha1.REJECT:
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
		default:
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

// processRateLimitPolicy processes RateLimit configuration and creates corresponding agentgateway policies
func processRateLimitPolicy(ctx krt.HandlerContext, gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension], trafficPolicy *v1alpha1.TrafficPolicy, policyName string, policyTarget *api.PolicyTarget) []ADPPolicy {
	var adpPolicies []ADPPolicy

	// Process local rate limiting if present
	if trafficPolicy.Spec.RateLimit.Local != nil {
		localPolicy := processLocalRateLimitPolicy(trafficPolicy, policyName, policyTarget)
		if localPolicy != nil {
			adpPolicies = append(adpPolicies, *localPolicy)
		} else {
			logger.Warn("failed to create local rate limit policy")
		}
	}

	// Process global rate limiting if present
	if trafficPolicy.Spec.RateLimit.Global != nil {
		globalPolicy := processGlobalRateLimitPolicy(ctx, gatewayExtensions, trafficPolicy, policyName, policyTarget)
		if globalPolicy != nil {
			adpPolicies = append(adpPolicies, *globalPolicy)
		} else {
			logger.Warn("failed to create global rate limit policy")
		}
	}

	return adpPolicies
}

// processLocalRateLimitPolicy processes local rate limiting configuration
func processLocalRateLimitPolicy(trafficPolicy *v1alpha1.TrafficPolicy, policyName string, policyTarget *api.PolicyTarget) *ADPPolicy {
	if trafficPolicy.Spec.RateLimit.Local.TokenBucket == nil {
		logger.Error("token bucket configuration is nil")
		return nil
	}

	tokenBucket := trafficPolicy.Spec.RateLimit.Local.TokenBucket

	// Validate configuration
	if tokenBucket.MaxTokens <= 0 {
		logger.Error("invalid max tokens value", "max_tokens", tokenBucket.MaxTokens)
		return nil
	}
	// Convert duration to seconds for agentgateway
	fillIntervalSeconds := uint32(tokenBucket.FillInterval.Duration.Seconds())
	if fillIntervalSeconds == 0 {
		fillIntervalSeconds = 1 // minimum 1 second
	}

	// Create local rate limit policy using the proper agentgateway API
	tokensPerFill := uint64(ptr.Deref(tokenBucket.TokensPerFill, 1))
	if tokensPerFill == 0 {
		tokensPerFill = 1
	}

	localRateLimitPolicy := &api.Policy{
		Name:   policyName + localRateLimitPolicySuffix,
		Target: policyTarget,
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_LocalRateLimit_{
				LocalRateLimit: &api.PolicySpec_LocalRateLimit{
					MaxTokens:     uint64(tokenBucket.MaxTokens),
					TokensPerFill: tokensPerFill,
					FillInterval:  &durationpb.Duration{Seconds: int64(fillIntervalSeconds)},
					Type:          api.PolicySpec_LocalRateLimit_REQUEST,
				},
			},
		},
	}

	return &ADPPolicy{Policy: localRateLimitPolicy}
}

func processGlobalRateLimitPolicy(
	ctx krt.HandlerContext,
	gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension],
	trafficPolicy *v1alpha1.TrafficPolicy,
	policyName string,
	policyTarget *api.PolicyTarget,
) *ADPPolicy {
	grl := trafficPolicy.Spec.RateLimit.Global
	if grl == nil {
		return nil
	}

	gwExt, err := lookupGatewayExtension(
		ctx, gatewayExtensions, grl.ExtensionRef, trafficPolicy.Namespace, v1alpha1.GatewayExtensionTypeRateLimit,
	)
	if err != nil {
		logger.Error("failed to lookup rate limit extension", "error", err)
		return nil
	}
	if gwExt.Spec.RateLimit == nil ||
		gwExt.Spec.RateLimit.GrpcService == nil ||
		gwExt.Spec.RateLimit.GrpcService.BackendRef == nil {
		logger.Error("rate limit extension missing grpcService.backendRef", "extension", gwExt.Name)
		return nil
	}

	// Build BackendReference for agentgateway (service/ns + port)
	agwRef, err := buildAGWServiceRef(gwExt.Spec.RateLimit.GrpcService.BackendRef, trafficPolicy.Namespace)
	if err != nil {
		logger.Error("failed to build AGW service reference", "error", err)
		return nil
	}

	// Translate descriptors
	descriptors := make([]*api.PolicySpec_RemoteRateLimit_Descriptor, 0, len(grl.Descriptors))
	for _, d := range grl.Descriptors {
		if adp := processRateLimitDescriptor(d); adp != nil {
			descriptors = append(descriptors, adp)
		}
	}

	// Build the RemoteRateLimit policy that agentgateway expects
	p := &api.Policy{
		Name:   policyName + globalRateLimitPolicySuffix,
		Target: policyTarget,
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_RemoteRateLimit_{
				RemoteRateLimit: &api.PolicySpec_RemoteRateLimit{
					Domain:      gwExt.Spec.RateLimit.Domain,
					Target:      agwRef,
					Descriptors: descriptors,
				},
			},
		},
	}

	return &ADPPolicy{Policy: p}
}

// getDescriptorEntryKey extracts the key from a rate limit descriptor entry
func getDescriptorEntryKey(entry v1alpha1.RateLimitDescriptorEntry) string {
	switch entry.Type {
	case v1alpha1.RateLimitDescriptorEntryTypeGeneric:
		if entry.Generic != nil {
			return entry.Generic.Key
		}
	case v1alpha1.RateLimitDescriptorEntryTypeHeader:
		if entry.Header != nil {
			return *entry.Header
		}
	case v1alpha1.RateLimitDescriptorEntryTypeRemoteAddress:
		return "remote_address"
	case v1alpha1.RateLimitDescriptorEntryTypePath:
		return "path"
	}
	return ""
}

func lookupGatewayExtension(ctx krt.HandlerContext, gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension], extensionRef v1alpha1.NamespacedObjectReference, defaultNamespace string, expectedType v1alpha1.GatewayExtensionType) (*v1alpha1.GatewayExtension, error) {
	extensionName := extensionRef.Name
	extensionNamespace := string(ptr.Deref(extensionRef.Namespace, ""))
	if extensionNamespace == "" {
		extensionNamespace = defaultNamespace
	}

	gwExtKey := fmt.Sprintf("%s/%s", extensionNamespace, extensionName)
	gwExt := krt.FetchOne(ctx, gatewayExtensions, krt.FilterKey(gwExtKey))

	if gwExt == nil {
		return nil, fmt.Errorf("gateway extension not found: %s", gwExtKey)
	}

	if (*gwExt).Spec.Type != expectedType {
		return nil, fmt.Errorf("gateway extension is not of type %s: %s", expectedType, gwExtKey)
	}

	return *gwExt, nil
}

func buildServiceHost(backendRef *gwv1.BackendRef, defaultNamespace string) (serviceName, namespace string, port uint32, err error) {
	if backendRef == nil {
		return "", "", 0, fmt.Errorf("backend reference is nil")
	}

	serviceName = string(backendRef.Name)
	if serviceName == "" {
		return "", "", 0, fmt.Errorf("service name is empty")
	}

	port = uint32(80) // default port
	if backendRef.Port != nil {
		port = uint32(*backendRef.Port)
	}

	namespace = defaultNamespace
	if backendRef.Namespace != nil {
		namespace = string(*backendRef.Namespace)
	}
	return serviceName, namespace, port, nil
}

func processRateLimitDescriptor(descriptor v1alpha1.RateLimitDescriptor) *api.PolicySpec_RemoteRateLimit_Descriptor {
	if len(descriptor.Entries) == 0 {
		return nil
	}

	entries := make([]*api.PolicySpec_RemoteRateLimit_Entry, 0, len(descriptor.Entries))

	for _, entry := range descriptor.Entries {
		// Map TrafficPolicy entry -> CEL expression or literal expected by the agent.
		var value string
		key := getDescriptorEntryKey(entry)
		switch entry.Type {
		case v1alpha1.RateLimitDescriptorEntryTypeGeneric:
			if entry.Generic != nil {
				// Constant literal -> quote for CEL
				value = strconv.Quote(entry.Generic.Value)
			}
		case v1alpha1.RateLimitDescriptorEntryTypeHeader:
			if entry.Header != nil {
				// Dynamic: header value expression
				value = celHeaderExpr(*entry.Header)
			}
		case v1alpha1.RateLimitDescriptorEntryTypeRemoteAddress:
			value = celRemoteIPExpr()
		case v1alpha1.RateLimitDescriptorEntryTypePath:
			value = celPathExpr()
		}
		if key != "" && value != "" {
			entries = append(entries, &api.PolicySpec_RemoteRateLimit_Entry{
				Key:   key,
				Value: value,
			})
		}
	}

	if len(entries) == 0 {
		return nil
	}
	return &api.PolicySpec_RemoteRateLimit_Descriptor{
		Entries: entries,
		Type:    api.PolicySpec_RemoteRateLimit_REQUESTS,
	}
}

// celHeaderExpr returns a CEL expression that reads a request header.
func celHeaderExpr(name string) string {
	// Convert to lowercase to match how HTTP headers are stored
	return fmt.Sprintf(`request.headers[%q]`, strings.ToLower(name))
}

// celRemoteIPExpr returns a CEL expression for the client IP.
func celRemoteIPExpr() string {
	return "source.address"
}

// celPathExpr returns a CEL expression for the request path (if supported in your env).
func celPathExpr() string {
	return "request.path"
}

// Returns an agentgateway BackendReference.Service in the "<ns>/<fqdn>" form + Port.
func buildAGWServiceRef(br *gwv1.BackendRef, defaultNS string) (*api.BackendReference, error) {
	if br == nil {
		return nil, fmt.Errorf("backendRef is nil")
	}
	name, ns, port, err := buildServiceHost(br, defaultNS)
	if err != nil {
		return nil, err
	}
	fqdn := kubeutils.ServiceFQDN(metav1.ObjectMeta{Namespace: ns, Name: name})
	return &api.BackendReference{
		Kind: &api.BackendReference_Service{Service: ns + "/" + fqdn},
		Port: port,
	}, nil
}

func toJSONValue(value string) (string, error) {
	if json.Valid([]byte(value)) {
		return value, nil
	}

	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		return "", fmt.Errorf("invalid JSON value: %s", value)
	}

	// Treat this as an unquoted string and marshal it to JSON
	marshaled, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(marshaled), nil
}
