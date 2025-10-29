package plugins

import (
	"errors"
	"fmt"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/sslutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	aiPolicySuffix               = ":ai"
	backendauthPolicySuffix      = ":backend-auth"
	tlsPolicySuffix              = ":tls"
	mcpAuthorizationPolicySuffix = ":mcp-authorization"
)

func translateBackendPolicyToAgw(
	ctx PolicyCtx,
	policy *v1alpha1.AgentgatewayPolicy,
	policyTarget *api.PolicyTarget,
) ([]AgwPolicy, error) {
	backend := policy.Spec.Backend
	if backend == nil {
		return nil, nil
	}
	agwPolicies := make([]AgwPolicy, 0)
	var errs []error

	policyName := getBackendPolicyName(policy.Namespace, policy.Name)

	if s := backend.HTTP; s != nil {
		pol, err := translateBackendHTTP(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing backend HTTP", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := backend.TLS; s != nil {
		pol, err := translateBackendTLS(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing backend TLS", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := backend.TCP; s != nil {
		pol, err := translateBackendTCP(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing backend TCP", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := backend.MCP; s != nil {
		pol, err := translateBackendMCPAuthorization(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing backend MCP Authorization", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := backend.AI; s != nil {
		pol, err := translateBackendAI(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing backend Tracing", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := backend.Auth; s != nil {
		pol, err := translateBackendAuth(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing backend Tracing", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	return agwPolicies, errors.Join(errs...)
}

func translateBackendTCP(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	// TODO
	return nil, nil
}
func translateBackendTLS(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	var errs []error

	// Build CA bundle from referenced ConfigMaps, if provided
	var caCert *wrapperspb.BytesValue
	if tls := policy.Spec.Backend.TLS; tls != nil && len(tls.CACertificateRefs) > 0 {
		var sb strings.Builder
		for _, ref := range tls.CACertificateRefs {
			nn := types.NamespacedName{Namespace: policy.Namespace, Name: ref.Name}
			cfgmap := krt.FetchOne(ctx.Krt, ctx.Collections.ConfigMaps, krt.FilterObjectName(nn))
			if cfgmap == nil {
				errs = append(errs, fmt.Errorf("ConfigMap %s not found", nn))
				continue
			}
			pem, err := sslutils.GetCACertFromConfigMap(ptr.Flatten(cfgmap))
			if err != nil {
				errs = append(errs, fmt.Errorf("error extracting CA cert from ConfigMap %s: %w", nn, err))
				continue
			}
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(pem)
		}
		if sb.Len() > 0 {
			caCert = wrapperspb.Bytes([]byte(sb.String()))
		}
	}

	// Map verify SANs to Hostname if provided (use first entry only)
	var hostname *wrapperspb.StringValue
	if tls := policy.Spec.Backend.TLS; tls != nil && len(tls.VerifySubjectAltNames) > 0 {
		hostname = wrapperspb.String(tls.VerifySubjectAltNames[0])
	}

	// Map insecure modes
	var insecure *wrapperspb.BoolValue
	if tls := policy.Spec.Backend.TLS; tls != nil && tls.InsecureSkipVerify != nil {
		switch *tls.InsecureSkipVerify {
		case v1alpha1.InsecureTLSModeAll:
			insecure = wrapperspb.Bool(true)
		case v1alpha1.InsecureTLSModeHostname:
			// Not directly supported in agentgateway API; fall back to default verification
		}
	}

	tlsPolicy := &api.Policy{
		Name:   policy.Namespace + "/" + policy.Name + tlsPolicySuffix + attachmentName(target),
		Target: target,
		Kind: &api.Policy_Backend{
			Backend: &api.BackendPolicySpec{
				Kind: &api.BackendPolicySpec_BackendTls{
					BackendTls: &api.BackendPolicySpec_BackendTLS{
						Root:     caCert,
						Cert:     nil,
						Key:      nil,
						Insecure: insecure,
						Hostname: hostname,
					},
				},
			}},
	}

	logger.Debug("generated TLS policy",
		"policy", policy.Name,
		"agentgateway_policy", tlsPolicy.Name)

	return []AgwPolicy{{Policy: tlsPolicy}}, errors.Join(errs...)
}
func translateBackendHTTP(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	// TODO
	return nil, nil
}

func translateBackendMCPAuthorization(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	auth := policy.Spec.Traffic.Authorization
	var allowPolicies, denyPolicies []string
	if auth.Action == v1alpha1.AuthorizationPolicyActionDeny {
		denyPolicies = append(denyPolicies, cast(auth.Policy.MatchExpressions)...)
	} else {
		allowPolicies = append(allowPolicies, cast(auth.Policy.MatchExpressions)...)
	}

	mcpPolicy := &api.Policy{
		Name:   policy.Namespace + "/" + policy.Name + mcpAuthorizationPolicySuffix + attachmentName(target),
		Target: target,
		Kind: &api.Policy_Backend{
			Backend: &api.BackendPolicySpec{
				Kind: &api.BackendPolicySpec_McpAuthorization_{
					McpAuthorization: &api.BackendPolicySpec_McpAuthorization{
						Allow: allowPolicies,
						Deny:  denyPolicies,
					},
				},
			}},
	}

	logger.Debug("generated MCP policy",
		"policy", policy.Name,
		"agentgateway_policy", mcpPolicy.Name)

	return []AgwPolicy{{Policy: mcpPolicy}}, nil
}

// translateBackendAI processes AI configuration and creates corresponding Agw policies
func translateBackendAI(ctx PolicyCtx, agwPolicy *v1alpha1.AgentgatewayPolicy, name string, policyTarget *api.PolicyTarget) ([]AgwPolicy, error) {
	var errs []error
	aiSpec := agwPolicy.Spec.Backend.AI

	translatedAIPolicy := &api.BackendPolicySpec_Ai{}
	if aiSpec.PromptEnrichment != nil {
		translatedAIPolicy.Prompts = processPromptEnrichment(aiSpec.PromptEnrichment)
	}

	for _, def := range aiSpec.Defaults {
		val, err := toJSONValue(def.Value)
		if err != nil {
			logger.Error("error parsing field value", "field", def.Field, "error", err)
			errs = append(errs, err)
			continue
		}
		if def.Override {
			if translatedAIPolicy.Overrides == nil {
				translatedAIPolicy.Overrides = make(map[string]string)
			}
			translatedAIPolicy.Overrides[def.Field] = val
		} else {
			if translatedAIPolicy.Defaults == nil {
				translatedAIPolicy.Defaults = make(map[string]string)
			}
			translatedAIPolicy.Defaults[def.Field] = val
		}
	}

	if aiSpec.PromptGuard != nil {
		if translatedAIPolicy.PromptGuard == nil {
			translatedAIPolicy.PromptGuard = &api.BackendPolicySpec_Ai_PromptGuard{}
		}
		if aiSpec.PromptGuard.Request != nil {
			translatedAIPolicy.PromptGuard.Request = processRequestGuard(ctx.Krt, ctx.Collections.Secrets, agwPolicy.Namespace, aiSpec.PromptGuard.Request)
		}

		if aiSpec.PromptGuard.Response != nil {
			translatedAIPolicy.PromptGuard.Response = processResponseGuard(aiSpec.PromptGuard.Response)
		}
	}

	if aiSpec.ModelAliases != nil {
		translatedAIPolicy.ModelAliases = aiSpec.ModelAliases
	}

	aiPolicy := &api.Policy{
		Name:   name + aiPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Kind: &api.Policy_Backend{
			Backend: &api.BackendPolicySpec{
				Kind: &api.BackendPolicySpec_Ai_{
					Ai: translatedAIPolicy,
				},
			},
		},
	}

	logger.Debug("generated AI policy",
		"policy", agwPolicy.Name,
		"agentgateway_policy", aiPolicy.Name)

	return []AgwPolicy{{Policy: aiPolicy}}, errors.Join(errs...)
}

func translateBackendAuth(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	var errs []error
	auth := policy.Spec.Backend.Auth

	if auth == nil {
		return nil, nil
	}

	var translatedAuth *api.BackendAuthPolicy

	if auth.InlineKey != nil && *auth.InlineKey != "" {
		translatedAuth = &api.BackendAuthPolicy{
			Kind: &api.BackendAuthPolicy_Key{
				Key: &api.Key{Secret: *auth.InlineKey},
			},
		}
	} else if auth.SecretRef != nil {
		// Resolve secret and extract Authorization value
		secret, err := kubeutils.GetSecret(ctx.Collections.Secrets, ctx.Krt, auth.SecretRef.Name, policy.Namespace)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get secret %s/%s: %w", policy.Namespace, auth.SecretRef.Name, err))
		} else {
			if authKey, ok := kubeutils.GetSecretAuth(secret); ok {
				translatedAuth = &api.BackendAuthPolicy{
					Kind: &api.BackendAuthPolicy_Key{
						Key: &api.Key{Secret: authKey},
					},
				}
			} else {
				errs = append(errs, fmt.Errorf("secret %s/%s missing Authorization value", policy.Namespace, auth.SecretRef.Name))
			}
		}
	} else {
		errs = append(errs, fmt.Errorf("backend auth requires either inline key or secretRef"))
	}

	if translatedAuth == nil {
		return nil, errors.Join(errs...)
	}

	authPolicy := &api.Policy{
		Name:   name + backendauthPolicySuffix + attachmentName(target),
		Target: target,
		Kind: &api.Policy_Backend{
			Backend: &api.BackendPolicySpec{
				Kind: &api.BackendPolicySpec_Auth{
					Auth: translatedAuth,
				},
			},
		},
	}
	logger.Debug("generated backend auth policy",
		"policy", policy.Name,
		"agentgateway_policy", authPolicy.Name)

	return []AgwPolicy{{Policy: authPolicy}}, errors.Join(errs...)
}
