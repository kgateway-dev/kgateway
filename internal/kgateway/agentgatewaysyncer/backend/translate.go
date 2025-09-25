package agentgatewaybackend

import (
	"errors"
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

var logger = logging.New("agentgateway/backend")

const (
	authPolicyPrefix = "auth"
)

// BuildAgwBackendIr translates a Backend to an AgwBackendIr
func BuildAgwBackendIr(
	krtctx krt.HandlerContext,
	secrets krt.Collection[*corev1.Secret],
	services krt.Collection[*corev1.Service],
	namespaces krt.Collection[*corev1.Namespace],
	backend *v1alpha1.Backend,
) *AgwBackendIr {
	backendIr := &AgwBackendIr{}

	switch backend.Spec.Type {
	case v1alpha1.BackendTypeStatic:
		staticIr, err := buildStaticIr(backend)
		if err != nil {
			backendIr.Errors = append(backendIr.Errors, err)
		}
		backendIr.StaticIr = staticIr

	case v1alpha1.BackendTypeAI:
		aiIr, err := buildAIIr(krtctx, backend, secrets)
		if err != nil {
			backendIr.Errors = append(backendIr.Errors, err)
		}
		backendIr.AIIr = aiIr

	case v1alpha1.BackendTypeMCP:
		mcpIr, err := buildMCPIr(krtctx, backend, services, namespaces)
		if err != nil {
			backendIr.Errors = append(backendIr.Errors, err)
		}
		backendIr.MCPIr = mcpIr

	default:
		backendIr.Errors = append(backendIr.Errors, fmt.Errorf("unsupported backend type: %s", backend.Spec.Type))
	}

	return backendIr
}

// buildStaticIr pre-resolves static backend configuration
func buildStaticIr(be *v1alpha1.Backend) (*StaticIr, error) {
	// TODO(jmcguire98): as of now agentgateway does not support multiple hosts for static backends
	// if we want to have similar behavior to envoy (load balancing across all hosts provided)
	// we will need to add support for this in agentgateway
	if len(be.Spec.Static.Hosts) > 1 {
		return nil, fmt.Errorf("multiple hosts are currently not supported for static backends in agentgateway")
	}
	if len(be.Spec.Static.Hosts) == 0 {
		return nil, fmt.Errorf("static backends must have at least one host")
	}

	backend := &api.Backend{
		Name: be.Namespace + "/" + be.Name,
		Kind: &api.Backend_Static{
			Static: &api.StaticBackend{
				Host: be.Spec.Static.Hosts[0].Host,
				Port: int32(be.Spec.Static.Hosts[0].Port),
			},
		},
	}

	return &StaticIr{
		Backend: backend,
	}, nil
}

func translateLLMProviderToProvider(krtctx krt.HandlerContext, llm *v1alpha1.LLMProvider, providerName string, secrets krt.Collection[*corev1.Secret], namespace string) (*api.AIBackend_Provider, *api.BackendAuthPolicy, error) {
	provider := &api.AIBackend_Provider{
		Name: providerName,
	}

	var auth *api.BackendAuthPolicy

	if llm.Host != nil {
		provider.HostOverride = &api.AIBackend_HostOverride{
			Host: *llm.Host,
			Port: int32(ptr.Deref(llm.Port, 443)), // Port is required when Host is set (CEL validated)
		}
	}

	if llm.Path != nil {
		if llm.Path.Full != nil {
			provider.PathOverride = &wrappers.StringValue{Value: *llm.Path.Full}
		}
	}

	if llm.AuthHeader != nil {
		logger.Warn("auth header override is not supported for agentgateway")
	}

	// Extract auth token and model based on provider
	if llm.OpenAI != nil {
		openai := &api.AIBackend_OpenAI{}
		if llm.OpenAI.Model != nil {
			openai.Model = &wrappers.StringValue{Value: *llm.OpenAI.Model}
		}
		provider.Provider = &api.AIBackend_Provider_Openai{
			Openai: openai,
		}
		auth = buildTranslatedAuthPolicy(krtctx, &llm.OpenAI.AuthToken, secrets, namespace)
	} else if llm.AzureOpenAI != nil {
		provider.Provider = &api.AIBackend_Provider_Openai{
			Openai: &api.AIBackend_OpenAI{},
		}
		auth = buildTranslatedAuthPolicy(krtctx, &llm.AzureOpenAI.AuthToken, secrets, namespace)
	} else if llm.Anthropic != nil {
		anthropic := &api.AIBackend_Anthropic{}
		if llm.Anthropic.Model != nil {
			anthropic.Model = &wrappers.StringValue{Value: *llm.Anthropic.Model}
		}
		provider.Provider = &api.AIBackend_Provider_Anthropic{
			Anthropic: anthropic,
		}
		auth = buildTranslatedAuthPolicy(krtctx, &llm.Anthropic.AuthToken, secrets, namespace)
	} else if llm.Gemini != nil {
		provider.Provider = &api.AIBackend_Provider_Gemini{
			Gemini: &api.AIBackend_Gemini{
				Model: &wrappers.StringValue{Value: llm.Gemini.Model},
			},
		}
		auth = buildTranslatedAuthPolicy(krtctx, &llm.Gemini.AuthToken, secrets, namespace)
	} else if llm.VertexAI != nil {
		provider.Provider = &api.AIBackend_Provider_Vertex{
			Vertex: &api.AIBackend_Vertex{
				Model:     &wrappers.StringValue{Value: llm.VertexAI.Model},
				Region:    llm.VertexAI.Location,
				ProjectId: llm.VertexAI.ProjectId,
			},
		}
		auth = buildTranslatedAuthPolicy(krtctx, &llm.VertexAI.AuthToken, secrets, namespace)
	} else if llm.Bedrock != nil {
		model := &wrappers.StringValue{
			Value: llm.Bedrock.Model,
		}
		region := llm.Bedrock.Region
		var guardrailIdentifier, guardrailVersion *wrappers.StringValue
		if llm.Bedrock.Guardrail != nil {
			guardrailIdentifier = &wrappers.StringValue{
				Value: llm.Bedrock.Guardrail.GuardrailIdentifier,
			}
			guardrailVersion = &wrappers.StringValue{
				Value: llm.Bedrock.Guardrail.GuardrailVersion,
			}
		}

		provider.Provider = &api.AIBackend_Provider_Bedrock{
			Bedrock: &api.AIBackend_Bedrock{
				Model:               model,
				Region:              region,
				GuardrailIdentifier: guardrailIdentifier,
				GuardrailVersion:    guardrailVersion,
			},
		}
		var err error
		auth, err = buildBedrockAuthPolicy(krtctx, region, llm.Bedrock.Auth, secrets, namespace)
		if err != nil {
			return nil, nil, err
		}
	} else {
		return nil, nil, fmt.Errorf("no supported LLM provider configured")
	}

	return provider, auth, nil
}

// createAuthPolicy creates an auth policy for a sub-backend target
func createAuthPolicy(authPolicy *api.BackendAuthPolicy, backendName, providerName string) *api.Policy {
	if authPolicy == nil {
		return nil
	}

	subBackendTarget := fmt.Sprintf("%s/%s", backendName, providerName)
	return &api.Policy{
		Name: fmt.Sprintf("%s-%s-%s", authPolicyPrefix, backendName, providerName),
		Target: &api.PolicyTarget{
			Kind: &api.PolicyTarget_SubBackend{
				SubBackend: subBackendTarget,
			},
		},
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_Auth{
				Auth: authPolicy,
			},
		},
	}
}

func buildAIIr(krtctx krt.HandlerContext, be *v1alpha1.Backend, secrets krt.Collection[*corev1.Secret]) (*AIIr, error) {
	backendName := utils.InternalBackendName(be.Namespace, be.Name, "")
	aiBackend := &api.AIBackend{
		ProviderGroups: []*api.AIBackend_ProviderGroup{},
	}
	var policies []*api.Policy
	providerIndex := 0

	if be.Spec.AI.LLM != nil {
		providerGroup := &api.AIBackend_ProviderGroup{
			Providers: []*api.AIBackend_Provider{},
		}

		// in a single provider case, use the fixed sub-backend name
		providerName := utils.SingularLLMProviderSubBackendName
		provider, authPolicy, err := translateLLMProviderToProvider(krtctx, be.Spec.AI.LLM, providerName, secrets, be.Namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to translate LLM provider: %w", err)
		}
		providerGroup.Providers = append(providerGroup.Providers, provider)

		if policy := createAuthPolicy(authPolicy, backendName, providerName); policy != nil {
			policies = append(policies, policy)
		}

		aiBackend.ProviderGroups = append(aiBackend.ProviderGroups, providerGroup)
	} else {
		for _, group := range be.Spec.AI.PriorityGroups {
			providerGroup := &api.AIBackend_ProviderGroup{}

			// Add all providers in this priority level to the same group
			for _, provider := range group.Providers {
				providerName := string(provider.Name)

				provider, authPolicy, err := translateLLMProviderToProvider(krtctx, &provider.LLMProvider, providerName, secrets, be.Namespace)
				if err != nil {
					return nil, fmt.Errorf("failed to translate provider in provider %s: %w", providerName, err)
				}
				providerGroup.Providers = append(providerGroup.Providers, provider)

				if policy := createAuthPolicy(authPolicy, backendName, providerName); policy != nil {
					policies = append(policies, policy)
				}
				providerIndex++
			}

			if len(providerGroup.Providers) > 0 {
				aiBackend.ProviderGroups = append(aiBackend.ProviderGroups, providerGroup)
			}
		}
	}

	if len(aiBackend.ProviderGroups) == 0 {
		return nil, fmt.Errorf("no valid AI provider groups were translated")
	}

	backend := &api.Backend{
		Name: backendName,
		Kind: &api.Backend_Ai{
			Ai: aiBackend,
		},
	}

	return &AIIr{
		Backend:  backend,
		Policies: policies,
	}, nil
}

// buildTranslatedAuthPolicy creates auth policy for the given auth token configuration
func buildTranslatedAuthPolicy(krtctx krt.HandlerContext, authToken *v1alpha1.SingleAuthToken, secrets krt.Collection[*corev1.Secret], namespace string) *api.BackendAuthPolicy {
	if authToken == nil {
		return nil
	}

	switch authToken.Kind {
	case v1alpha1.SecretRef:
		if authToken.SecretRef == nil {
			return nil
		}

		// Get secret using the SecretIndex
		secret, err := kubeutils.GetSecret(secrets, krtctx, authToken.SecretRef.Name, namespace)
		if err != nil {
			// Return nil auth policy if secret not found - this will be handled upstream
			// TODO(npolshak): Add backend status errors https://github.com/kgateway-dev/kgateway/issues/11966
			return nil
		}

		authKey, exists := kubeutils.GetSecretAuth(secret)
		if !exists {
			return nil
		}

		return &api.BackendAuthPolicy{
			Kind: &api.BackendAuthPolicy_Key{
				Key: &api.Key{Secret: authKey},
			},
		}
	case v1alpha1.Inline:
		if authToken.Inline == nil {
			return nil
		}
		return &api.BackendAuthPolicy{
			Kind: &api.BackendAuthPolicy_Key{
				Key: &api.Key{Secret: *authToken.Inline},
			},
		}
	case v1alpha1.Passthrough:
		return &api.BackendAuthPolicy{
			Kind: &api.BackendAuthPolicy_Passthrough{
				Passthrough: &api.Passthrough{},
			},
		}
	default:
		return nil
	}
}

// buildMCPIr pre-resolves MCP backend configuration including service discovery
func buildMCPIr(krtctx krt.HandlerContext, be *v1alpha1.Backend, services krt.Collection[*corev1.Service], namespaces krt.Collection[*corev1.Namespace]) (*MCPIr, error) {
	if be.Spec.MCP == nil {
		return nil, fmt.Errorf("mcp backend spec must not be nil for MCP backend type")
	}

	var mcpTargets []*api.MCPTarget
	var backends []*api.Backend
	serviceEndpoints := make(map[string]*ServiceEndpoint)

	// Process each target selector
	for _, targetSelector := range be.Spec.MCP.Targets {
		// Handle static targets
		if targetSelector.Static != nil {
			// Since policies can target specific targets within an MCP backend using SectionName,
			// the key for the target must include the Backend Name to prevent collisions with
			// policies targeting the entire Backend that have the same name as the target
			staticBackendRef := utils.InternalMCPStaticBackendName(be.Namespace, be.Name, string(targetSelector.Name))
			staticBackend := &api.Backend{
				Name: staticBackendRef,
				Kind: &api.Backend_Static{
					Static: &api.StaticBackend{
						Host: targetSelector.Static.Host,
						Port: targetSelector.Static.Port,
					},
				},
			}
			backends = append(backends, staticBackend)

			mcpTarget := &api.MCPTarget{
				Name: string(targetSelector.Name),
				Backend: &api.BackendReference{
					Kind: &api.BackendReference_Backend{
						Backend: staticBackendRef,
					},
					Port: uint32(targetSelector.Static.Port), //nolint:gosec // G115: Port is validated by kubebuilder to be 1-65535
				},
				Path: ptr.Deref(targetSelector.Static.Path, ""),
			}

			// Convert protocol if specified
			switch ptr.Deref(targetSelector.Static.Protocol, v1alpha1.MCPProtocol("")) {
			case v1alpha1.MCPProtocolSSE:
				mcpTarget.Protocol = api.MCPTarget_SSE
			case v1alpha1.MCPProtocolStreamableHTTP:
				mcpTarget.Protocol = api.MCPTarget_STREAMABLE_HTTP
			default:
				mcpTarget.Protocol = api.MCPTarget_UNDEFINED
			}

			mcpTargets = append(mcpTargets, mcpTarget)

			// Store static endpoint info
			serviceEndpoints[staticBackendRef] = &ServiceEndpoint{
				Host:      targetSelector.Static.Host,
				Port:      targetSelector.Static.Port,
				Namespace: be.Namespace,
			}
		}

		// Handle service selectors
		if targetSelector.Selector != nil {
			// Build filters for service discovery
			// Krt only allows 1 filter per type, so we build a composite filter here
			generic := func(svc any) bool {
				return true
			}
			addFilter := func(nf func(svc any) bool) {
				og := generic
				generic = func(svc any) bool {
					return nf(svc) && og(svc)
				}
			}

			// Apply service label selector
			if targetSelector.Selector.Service != nil {
				serviceSelector, err := metav1.LabelSelectorAsSelector(targetSelector.Selector.Service)
				if err != nil {
					return nil, fmt.Errorf("invalid service selector: %w", err)
				}
				if !serviceSelector.Empty() {
					addFilter(func(obj any) bool {
						service := obj.(*corev1.Service)
						return serviceSelector.Matches(labels.Set(service.Labels))
					})
				}
			}

			// Apply namespace selector
			if targetSelector.Selector.Namespace != nil {
				namespaceSelector, err := metav1.LabelSelectorAsSelector(targetSelector.Selector.Namespace)
				if err != nil {
					return nil, fmt.Errorf("invalid namespace selector: %w", err)
				}
				if !namespaceSelector.Empty() {
					// Get all namespaces and find those matching the selector
					allNamespaces := krt.Fetch(krtctx, namespaces)
					matchingNamespaces := make(map[string]bool)
					for _, ns := range allNamespaces {
						if namespaceSelector.Matches(labels.Set(ns.Labels)) {
							matchingNamespaces[ns.Name] = true
						}
					}
					// Filter services to only those in matching namespaces
					addFilter(func(obj any) bool {
						service := obj.(*corev1.Service)
						return matchingNamespaces[service.Namespace]
					})
				}
			} else {
				// If no namespace selector, limit to same namespace as backend
				addFilter(func(obj any) bool {
					service := obj.(*corev1.Service)
					return service.Namespace == be.Namespace
				})
			}

			// Fetch matching services
			matchingServices := krt.Fetch(krtctx, services, krt.FilterGeneric(generic))

			// Create MCP targets for each matching service
			for _, service := range matchingServices {
				for _, port := range service.Spec.Ports {
					appProtocol := ptr.Deref(port.AppProtocol, "")
					if appProtocol != mcpProtocol && appProtocol != mcpProtocolSSE {
						// not a valid MCP protocol
						continue
					}
					targetName := service.Name + fmt.Sprintf("-%d", port.Port)
					if port.Name != "" {
						targetName = service.Name + "-" + port.Name
					}

					svcHostname := kubeutils.ServiceFQDN(service.ObjectMeta)

					mcpTarget := &api.MCPTarget{
						Name: targetName,
						Backend: &api.BackendReference{
							Kind: &api.BackendReference_Service{
								Service: service.Namespace + "/" + svcHostname,
							},
							Port: uint32(port.Port), //nolint:gosec // G115: Kubernetes service ports are always positive
						},
						Protocol: toMCPProtocol(appProtocol),
						Path:     service.Annotations[apiannotations.MCPServiceHTTPPath],
					}

					mcpTargets = append(mcpTargets, mcpTarget)

					// Store service endpoint info
					serviceKey := service.Namespace + "/" + service.Name
					serviceEndpoints[serviceKey] = &ServiceEndpoint{
						Host:      svcHostname,
						Port:      port.Port,
						Service:   service,
						Namespace: service.Namespace,
					}
				}
			}
		}
	}

	// Create the main MCP backend
	mcpBackend := &api.Backend{
		Name: be.Namespace + "/" + be.Name,
		Kind: &api.Backend_Mcp{
			Mcp: &api.MCPBackend{
				Targets: mcpTargets,
			},
		},
	}
	backends = append(backends, mcpBackend)

	return &MCPIr{
		Backends:         backends,
		ServiceEndpoints: serviceEndpoints,
	}, nil
}

func toMCPProtocol(appProtocol string) api.MCPTarget_Protocol {
	switch appProtocol {
	case mcpProtocol:
		return api.MCPTarget_STREAMABLE_HTTP

	case mcpProtocolSSE:
		return api.MCPTarget_SSE

	default:
		// should never happen since this function is only invoked for valid MCP protocols
		return api.MCPTarget_UNDEFINED
	}
}

func buildBedrockAuthPolicy(krtctx krt.HandlerContext, region string, auth *v1alpha1.AwsAuth, secrets krt.Collection[*corev1.Secret], namespace string) (*api.BackendAuthPolicy, error) {
	var errs []error
	if auth == nil {
		logger.Warn("using implicit AWS auth for AI backend")
		return &api.BackendAuthPolicy{
			Kind: &api.BackendAuthPolicy_Aws{
				Aws: &api.Aws{
					Kind: &api.Aws_Implicit{
						Implicit: &api.AwsImplicit{},
					},
				},
			},
		}, nil
	}

	switch auth.Type {
	case v1alpha1.AwsAuthTypeSecret:
		if auth.SecretRef == nil {
			return nil, nil
		}

		// Get secret using the SecretIndex
		secret, err := kubeutils.GetSecret(secrets, krtctx, auth.SecretRef.Name, namespace)
		if err != nil {
			// Return nil auth policy if secret not found - this will be handled upstream
			// TODO(npolshak): Add backend status errors https://github.com/kgateway-dev/kgateway/issues/11966
			return nil, err
		}

		var accessKeyId, secretAccessKey string
		var sessionToken *string

		// Extract access key
		if value, exists := kubeutils.GetSecretValue(secret, wellknown.AccessKey); !exists {
			errs = append(errs, errors.New("accessKey is missing or not a valid string"))
		} else {
			accessKeyId = value
		}

		// Extract secret key
		if value, exists := kubeutils.GetSecretValue(secret, wellknown.SecretKey); !exists {
			errs = append(errs, errors.New("secretKey is missing or not a valid string"))
		} else {
			secretAccessKey = value
		}

		// Extract session token (optional)
		if value, exists := kubeutils.GetSecretValue(secret, wellknown.SessionToken); exists {
			sessionToken = ptr.To(value)
		}

		return &api.BackendAuthPolicy{
			Kind: &api.BackendAuthPolicy_Aws{
				Aws: &api.Aws{
					Kind: &api.Aws_ExplicitConfig{
						ExplicitConfig: &api.AwsExplicitConfig{
							AccessKeyId:     accessKeyId,
							SecretAccessKey: secretAccessKey,
							SessionToken:    sessionToken,
							Region:          region,
						},
					},
				},
			},
		}, errors.Join(errs...)
	default:
		errs = append(errs, errors.New("unknown AWS auth type"))
		return nil, errors.Join(errs...)
	}
}
