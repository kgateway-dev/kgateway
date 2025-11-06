package plugins

import (
	"log/slog"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

func processRequestGuard(krtctx krt.HandlerContext, secrets krt.Collection[*corev1.Secret], namespace string, req *v1alpha1.PromptguardRequest) *api.BackendPolicySpec_Ai_RequestGuard {
	if req == nil {
		return nil
	}

	pgReq := &api.BackendPolicySpec_Ai_RequestGuard{
		Webhook:          processWebhook(req.Webhook),
		Regex:            processRegex(req.Regex, req.CustomResponse),
		OpenaiModeration: processModeration(krtctx, secrets, namespace, req.Moderation),
	}

	if req.CustomResponse != nil {
		pgReq.Rejection = &api.BackendPolicySpec_Ai_RequestRejection{
			Body:   []byte(*req.CustomResponse.Message),
			Status: uint32(*req.CustomResponse.StatusCode), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		}
	}

	return pgReq
}

func processResponseGuard(resp *v1alpha1.PromptguardResponse) *api.BackendPolicySpec_Ai_ResponseGuard {
	return &api.BackendPolicySpec_Ai_ResponseGuard{
		Webhook: processWebhook(resp.Webhook),
		Regex:   processRegex(resp.Regex, nil),
	}
}

func processPromptEnrichment(enrichment *v1alpha1.AIPromptEnrichment) *api.BackendPolicySpec_Ai_PromptEnrichment {
	pgPromptEnrichment := &api.BackendPolicySpec_Ai_PromptEnrichment{}

	// Add prepend messages
	for _, msg := range enrichment.Prepend {
		pgPromptEnrichment.Prepend = append(pgPromptEnrichment.Prepend, &api.BackendPolicySpec_Ai_Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Add append messages
	for _, msg := range enrichment.Append {
		pgPromptEnrichment.Append = append(pgPromptEnrichment.Append, &api.BackendPolicySpec_Ai_Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return pgPromptEnrichment
}

func processWebhook(webhook *v1alpha1.Webhook) *api.BackendPolicySpec_Ai_Webhook {
	if webhook == nil {
		return nil
	}

	w := &api.BackendPolicySpec_Ai_Webhook{
		Host: webhook.Host.Host,
		Port: uint32(webhook.Host.Port), //nolint:gosec // G115: webhook port is validated to be valid port range
	}

	if len(webhook.ForwardHeaderMatches) > 0 {
		headers := make([]*api.HeaderMatch, 0, len(webhook.ForwardHeaderMatches))
		for _, match := range webhook.ForwardHeaderMatches {
			switch ptr.OrDefault(match.Type, gwv1.HeaderMatchExact) {
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

func processBuiltinRegexRule(builtin v1alpha1.BuiltIn, logger *slog.Logger) *api.BackendPolicySpec_Ai_RegexRule {
	builtinValue, ok := api.BackendPolicySpec_Ai_BuiltinRegexRule_value[string(builtin)]
	if !ok {
		logger.Warn("unknown builtin regex rule", "builtin", builtin)
		builtinValue = int32(api.BackendPolicySpec_Ai_BUILTIN_UNSPECIFIED)
	}
	return &api.BackendPolicySpec_Ai_RegexRule{
		Kind: &api.BackendPolicySpec_Ai_RegexRule_Builtin{
			Builtin: api.BackendPolicySpec_Ai_BuiltinRegexRule(builtinValue),
		},
	}
}

func processNamedRegexRule(pattern, name string) *api.BackendPolicySpec_Ai_RegexRule {
	return &api.BackendPolicySpec_Ai_RegexRule{
		Kind: &api.BackendPolicySpec_Ai_RegexRule_Regex{
			Regex: &api.BackendPolicySpec_Ai_NamedRegex{
				Pattern: pattern,
				Name:    name,
			},
		},
	}
}

func processRegex(regex *v1alpha1.Regex, customResponse *v1alpha1.CustomResponse) *api.BackendPolicySpec_Ai_RegexRules {
	if regex == nil {
		return nil
	}

	rules := &api.BackendPolicySpec_Ai_RegexRules{}
	if regex.Action != nil {
		rules.Action = &api.BackendPolicySpec_Ai_Action{}
		switch *regex.Action {
		case v1alpha1.MASK:
			rules.Action.Kind = api.BackendPolicySpec_Ai_MASK
		case v1alpha1.REJECT:
			rules.Action.Kind = api.BackendPolicySpec_Ai_REJECT
			rules.Action.RejectResponse = &api.BackendPolicySpec_Ai_RequestRejection{}
			if customResponse != nil {
				if customResponse.Message != nil {
					rules.Action.RejectResponse.Body = []byte(*customResponse.Message)
				}
				if customResponse.StatusCode != nil {
					rules.Action.RejectResponse.Status = uint32(*customResponse.StatusCode) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
				}
			}
		default:
			logger.Warn("unsupported regex action", "action", *regex.Action)
			rules.Action.Kind = api.BackendPolicySpec_Ai_ACTION_UNSPECIFIED
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

func processModeration(krtctx krt.HandlerContext, secrets krt.Collection[*corev1.Secret], namespace string, moderation *v1alpha1.Moderation) *api.BackendPolicySpec_Ai_Moderation {
	// right now we only support OpenAI moderation, so we can return nil if the moderation is nil or the OpenAIModeration is nil
	if moderation == nil || moderation.OpenAIModeration == nil {
		return nil
	}

	pgModeration := &api.BackendPolicySpec_Ai_Moderation{}

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
			// Resolve the actual secret value from Kubernetes
			secret, err := kubeutils.GetSecret(secrets, krtctx, moderation.OpenAIModeration.AuthToken.SecretRef.Name, namespace)
			if err != nil {
				logger.Error("failed to get secret for OpenAI moderation", "secret", moderation.OpenAIModeration.AuthToken.SecretRef.Name, "namespace", namespace, "error", err)
				return nil
			}

			authKey, exists := kubeutils.GetSecretAuth(secret)
			if !exists {
				logger.Error("secret does not contain valid Authorization value", "secret", moderation.OpenAIModeration.AuthToken.SecretRef.Name, "namespace", namespace)
				return nil
			}

			pgModeration.Auth = &api.BackendAuthPolicy{
				Kind: &api.BackendAuthPolicy_Key{
					Key: &api.Key{
						Secret: authKey,
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
