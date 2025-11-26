package plugins

import (
	"errors"
	"fmt"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/sslutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	aiPolicySuffix               = ":ai"
	backendTlsPolicySuffix       = ":backend-tls"
	backendauthPolicySuffix      = ":backend-auth"
	tlsPolicySuffix              = ":tls"
	backendHttpPolicySuffix      = ":backend-http"
	mcpAuthorizationPolicySuffix = ":mcp-authorization"
)

func TranslateInlineBackendPolicy(
	ctx PolicyCtx,
	namespace string,
	policy *v1alpha1.AgentgatewayPolicyBackendFull,
) ([]*api.BackendPolicySpec, error) {
	dummy := &v1alpha1.AgentgatewayPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inline_policy",
			Namespace: namespace,
		},
		Spec: v1alpha1.AgentgatewayPolicySpec{Backend: policy},
	}
	res, err := translateBackendPolicyToAgw(ctx, dummy, nil)
	return slices.MapFilter(res, func(e AgwPolicy) **api.BackendPolicySpec {
		return ptr.Of(e.Policy.GetBackend())
	}), err
}

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
		pol := translateBackendHTTP(policy, policyTarget)
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := backend.TLS; s != nil {
		pol, err := translateBackendTLS(ctx, policy, policyTarget)
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
		pol := translateBackendMCPAuthorization(policy, policyTarget)
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := backend.AI; s != nil {
		pol, err := translateBackendAI(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing backend AI", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := backend.Auth; s != nil {
		pol, err := translateBackendAuth(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing backend Auth", "err", err)
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

func translateBackendTLS(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, target *api.PolicyTarget) ([]AgwPolicy, error) {
	var errs []error
	tls := policy.Spec.Backend.TLS

	p := &api.BackendPolicySpec_BackendTLS{}

	if len(tls.MtlsCertificateRef) > 0 {
		// Currently we only support one, and enforce this in the API
		mtls := tls.MtlsCertificateRef[0]
		nn := types.NamespacedName{
			Namespace: policy.Namespace,
			Name:      mtls.Name,
		}
		scrt := ptr.Flatten(krt.FetchOne(ctx.Krt, ctx.Collections.Secrets, krt.FilterObjectName(nn)))
		if scrt == nil {
			errs = append(errs, fmt.Errorf("secret %s not found", nn))
		} else {
			if _, err := sslutils.ValidateTlsSecretData(nn.Name, nn.Namespace, scrt.Data); err != nil {
				errs = append(errs, fmt.Errorf("secret %v contains invalid certificate: %v", nn, err))
			}
			p.Cert = wrapperspb.Bytes(scrt.Data[corev1.TLSCertKey])
			p.Key = wrapperspb.Bytes(scrt.Data[corev1.TLSPrivateKeyKey])
			if ca, f := scrt.Data[corev1.ServiceAccountRootCAKey]; f {
				p.Root = wrapperspb.Bytes(ca)
			}
		}
	}

	// Build CA bundle from referenced ConfigMaps, if provided
	// If we were using mTLS, we may be overriding the previously set p.Root -- this is intended
	if len(tls.CACertificateRefs) > 0 {
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
		// TODO: if none, submit something to make agentgateway reject requests instead of fail open.
		if sb.Len() > 0 {
			p.Root = wrapperspb.Bytes([]byte(sb.String()))
		}
	}

	if len(tls.VerifySubjectAltNames) > 0 {
		p.VerifySubjectAltNames = tls.VerifySubjectAltNames
	}
	p.Hostname = stringPb(tls.Sni)

	if tls.InsecureSkipVerify != nil {
		switch *tls.InsecureSkipVerify {
		case v1alpha1.InsecureTLSModeAll:
			p.Verification = api.BackendPolicySpec_BackendTLS_INSECURE_ALL
		case v1alpha1.InsecureTLSModeHostname:
			p.Verification = api.BackendPolicySpec_BackendTLS_INSECURE_HOST
		}
	}

	if tls.AlpnProtocols != nil {
		p.Alpn = &api.Alpn{Protocols: *tls.AlpnProtocols}
	}

	tlsPolicy := &api.Policy{
		Name:   policy.Namespace + "/" + policy.Name + tlsPolicySuffix + attachmentName(target),
		Target: target,
		Kind: &api.Policy_Backend{
			Backend: &api.BackendPolicySpec{
				Kind: &api.BackendPolicySpec_BackendTls{
					BackendTls: p,
				},
			}},
	}

	logger.Debug("generated TLS policy",
		"policy", policy.Name,
		"agentgateway_policy", tlsPolicy.Name)

	return []AgwPolicy{{Policy: tlsPolicy}}, errors.Join(errs...)
}

func translateBackendHTTP(policy *v1alpha1.AgentgatewayPolicy, target *api.PolicyTarget) []AgwPolicy {
	http := policy.Spec.Backend.HTTP
	p := &api.BackendPolicySpec_BackendHTTP{}
	if v := http.Version; v != nil {
		switch *v {
		case v1alpha1.HTTPVersion1:
			p.Version = api.BackendPolicySpec_BackendHTTP_HTTP1
		case v1alpha1.HTTPVersion2:
			p.Version = api.BackendPolicySpec_BackendHTTP_HTTP2
		}
	}
	tp := &api.Policy{
		Name:   policy.Namespace + "/" + policy.Name + backendHttpPolicySuffix + attachmentName(target),
		Target: target,
		Kind: &api.Policy_Backend{
			Backend: &api.BackendPolicySpec{
				Kind: &api.BackendPolicySpec_BackendHttp{
					BackendHttp: p,
				},
			}},
	}
	logger.Debug("generated HTTP policy",
		"policy", policy.Name,
		"agentgateway_policy", tp.Name)

	return []AgwPolicy{{Policy: tp}}
}

func translateBackendMCPAuthorization(policy *v1alpha1.AgentgatewayPolicy, target *api.PolicyTarget) []AgwPolicy {
	backend := policy.Spec.Backend
	if backend == nil || backend.MCP == nil || backend.MCP.Authorization == nil {
		return nil
	}
	auth := backend.MCP.Authorization
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

	logger.Debug("generated MCPBackend policy",
		"policy", policy.Name,
		"agentgateway_policy", mcpPolicy.Name)

	return []AgwPolicy{{Policy: mcpPolicy}}
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
		if translatedAIPolicy.Defaults == nil {
			translatedAIPolicy.Defaults = make(map[string]string)
		}
		translatedAIPolicy.Defaults[def.Field] = val
	}

	for _, def := range aiSpec.Overrides {
		val, err := toJSONValue(def.Value)
		if err != nil {
			logger.Error("error parsing field value", "field", def.Field, "error", err)
			errs = append(errs, err)
			continue
		}
		if translatedAIPolicy.Overrides == nil {
			translatedAIPolicy.Overrides = make(map[string]string)
		}
		translatedAIPolicy.Overrides[def.Field] = val
	}

	if aiSpec.PromptGuard != nil {
		if translatedAIPolicy.PromptGuard == nil {
			translatedAIPolicy.PromptGuard = &api.BackendPolicySpec_Ai_PromptGuard{}
		}
		if aiSpec.PromptGuard.Request != nil {
			r, err := processRequestGuard(ctx, agwPolicy.Namespace, aiSpec.PromptGuard.Request)
			if err != nil {
				logger.Error("error parsing request prompt guard", "error", err)
				errs = append(errs, err)
			} else {
				translatedAIPolicy.PromptGuard.Request = r
			}
		}

		if aiSpec.PromptGuard.Response != nil {
			r, err := processResponseGuard(ctx, agwPolicy.Namespace, aiSpec.PromptGuard.Response)
			if err != nil {
				logger.Error("error parsing response prompt guard", "error", err)
				errs = append(errs, err)
			} else {
				translatedAIPolicy.PromptGuard.Response = r
			}
		}
	}

	if aiSpec.ModelAliases != nil {
		translatedAIPolicy.ModelAliases = aiSpec.ModelAliases
	}

	if aiSpec.PromptCaching != nil {
		translatedAIPolicy.PromptCaching = &api.BackendPolicySpec_Ai_PromptCaching{
			CacheSystem:   aiSpec.PromptCaching.CacheSystem,
			CacheMessages: aiSpec.PromptCaching.CacheMessages,
			CacheTools:    aiSpec.PromptCaching.CacheTools,
		}
		translatedAIPolicy.PromptCaching.MinTokens = ptr.Of(uint32(aiSpec.PromptCaching.MinTokens)) //nolint:gosec // G115: MinTokens is validated by kubebuilder to be >= 0
	}

	if aiSpec.Routes != nil {
		r := make(map[string]api.BackendPolicySpec_Ai_RouteType)
		for path, routeType := range aiSpec.Routes {
			r[path] = translateRouteType(routeType)
		}
		// TODO: AGW support
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

// translateRouteType converts kgateway RouteType to agentgateway proto RouteType
func translateRouteType(rt v1alpha1.RouteType) api.BackendPolicySpec_Ai_RouteType {
	switch rt {
	case v1alpha1.RouteTypeCompletions:
		return api.BackendPolicySpec_Ai_COMPLETIONS
	case v1alpha1.RouteTypeMessages:
		return api.BackendPolicySpec_Ai_MESSAGES
	case v1alpha1.RouteTypeModels:
		return api.BackendPolicySpec_Ai_MODELS
	case v1alpha1.RouteTypePassthrough:
		return api.BackendPolicySpec_Ai_PASSTHROUGH
	case v1alpha1.RouteTypeResponses:
		return api.BackendPolicySpec_Ai_RESPONSES
	case v1alpha1.RouteTypeAnthropicTokenCount:
		return api.BackendPolicySpec_Ai_ANTHROPIC_TOKEN_COUNT
	default:
		// Default to completions if unknown type
		return api.BackendPolicySpec_Ai_COMPLETIONS
	}
}

func stringPb(model *string) *wrapperspb.StringValue {
	if model == nil {
		return nil
	}
	return wrapperspb.String(*model)
}
