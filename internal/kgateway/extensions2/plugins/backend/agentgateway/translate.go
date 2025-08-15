package agentgatewaybackend

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/agentgateway/agentgateway/go/api"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

// BuildAgentGatewayBackendIr translates a Backend to an AgentGatewayBackendIr
func BuildAgentGatewayBackendIr(
	krtctx krt.HandlerContext,
	secrets *krtcollections.SecretIndex,
	services krt.Collection[*corev1.Service],
	namespaces krt.Collection[krtcollections.NamespaceMetadata],
	i *v1alpha1.Backend,
) *AgentGatewayBackendIr {
	backendIr := &AgentGatewayBackendIr{}

	switch i.Spec.Type {
	case v1alpha1.BackendTypeStatic:
		staticIr, err := buildStaticIr(i)
		if err != nil {
			backendIr.Errors = append(backendIr.Errors, err)
		}
		backendIr.StaticIr = staticIr

	case v1alpha1.BackendTypeAI:
		aiIr, err := buildAIIr(krtctx, i, secrets)
		if err != nil {
			backendIr.Errors = append(backendIr.Errors, err)
		}
		backendIr.AIIr = aiIr

	case v1alpha1.BackendTypeMCP:
		mcpIr, err := buildMCPIr(krtctx, i, services, namespaces)
		if err != nil {
			backendIr.Errors = append(backendIr.Errors, err)
		}
		backendIr.MCPIr = mcpIr

	default:
		backendIr.Errors = append(backendIr.Errors, fmt.Errorf("unsupported backend type: %s", i.Spec.Type))
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

// buildAIIr pre-resolves AI backend configuration including secrets
func buildAIIr(krtctx krt.HandlerContext, be *v1alpha1.Backend, secrets *krtcollections.SecretIndex) (*AIIr, error) {
	if be.Spec.AI == nil {
		return nil, fmt.Errorf("ai backend spec must not be nil for AI backend type")
	}

	var llm *v1alpha1.LLMProvider
	if be.Spec.AI.LLM != nil {
		llm = be.Spec.AI.LLM
	} else if be.Spec.AI.MultiPool != nil && len(be.Spec.AI.MultiPool.Priorities) > 0 &&
		len(be.Spec.AI.MultiPool.Priorities[0].Pool) > 0 {
		// For MultiPool, use the first provider from the first priority pool
		llm = &be.Spec.AI.MultiPool.Priorities[0].Pool[0]
	} else {
		return nil, fmt.Errorf("AI backend has no valid LLM or MultiPool configuration")
	}

	aiBackend := &api.AIBackend{}
	var auth *api.BackendAuthPolicy

	// Extract auth token and model based on provider
	if llm.Provider.OpenAI != nil {
		openai := &api.AIBackend_OpenAI{}
		if llm.Provider.OpenAI.Model != nil {
			openai.Model = &wrappers.StringValue{Value: *llm.Provider.OpenAI.Model}
		}
		aiBackend.Provider = &api.AIBackend_Openai{
			Openai: openai,
		}
		auth = buildTranslatedAuthPolicy(krtctx, &llm.Provider.OpenAI.AuthToken, secrets, be.Namespace)
	} else if llm.Provider.AzureOpenAI != nil {
		aiBackend.Provider = &api.AIBackend_Openai{
			Openai: &api.AIBackend_OpenAI{},
		}
		auth = buildTranslatedAuthPolicy(krtctx, &llm.Provider.AzureOpenAI.AuthToken, secrets, be.Namespace)
	} else if llm.Provider.Anthropic != nil {
		anthropic := &api.AIBackend_Anthropic{}
		if llm.Provider.Anthropic.Model != nil {
			anthropic.Model = &wrappers.StringValue{Value: *llm.Provider.Anthropic.Model}
		}
		aiBackend.Provider = &api.AIBackend_Anthropic_{
			Anthropic: anthropic,
		}
		auth = buildTranslatedAuthPolicy(krtctx, &llm.Provider.Anthropic.AuthToken, secrets, be.Namespace)
	} else if llm.Provider.Gemini != nil {
		aiBackend.Provider = &api.AIBackend_Gemini_{
			Gemini: &api.AIBackend_Gemini{
				Model: &wrappers.StringValue{Value: llm.Provider.Gemini.Model},
			},
		}
		auth = buildTranslatedAuthPolicy(krtctx, &llm.Provider.Gemini.AuthToken, secrets, be.Namespace)
	} else if llm.Provider.VertexAI != nil {
		aiBackend.Provider = &api.AIBackend_Vertex_{
			Vertex: &api.AIBackend_Vertex{
				Model: &wrappers.StringValue{Value: llm.Provider.VertexAI.Model},
			},
		}
		auth = buildTranslatedAuthPolicy(krtctx, &llm.Provider.VertexAI.AuthToken, secrets, be.Namespace)
	} else if llm.Provider.Bedrock != nil {
		model := &wrappers.StringValue{
			Value: llm.Provider.Bedrock.Model,
		}
		region := llm.Provider.Bedrock.Region
		var guardrailIdentifier, guardrailVersion *wrappers.StringValue
		if llm.Provider.Bedrock.Guardrail != nil {
			guardrailIdentifier = &wrappers.StringValue{
				Value: llm.Provider.Bedrock.Guardrail.GuardrailIdentifier,
			}
			guardrailVersion = &wrappers.StringValue{
				Value: llm.Provider.Bedrock.Guardrail.GuardrailVersion,
			}
		}

		aiBackend.Provider = &api.AIBackend_Bedrock_{
			Bedrock: &api.AIBackend_Bedrock{
				Model:               model,
				Region:              region,
				GuardrailIdentifier: guardrailIdentifier,
				GuardrailVersion:    guardrailVersion,
			},
		}
		var err error
		auth, err = buildBedrockAuthPolicy(krtctx, region, llm.Provider.Bedrock.Auth, secrets, be.Namespace)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("no supported LLM provider configured")
	}

	return &AIIr{
		Backend:    aiBackend,
		AuthPolicy: auth,
		Name:       be.Namespace + "/" + be.Name,
	}, nil
}

// buildTranslatedAuthPolicy creates auth policy for the given auth token configuration
func buildTranslatedAuthPolicy(krtctx krt.HandlerContext, authToken *v1alpha1.SingleAuthToken, secrets *krtcollections.SecretIndex, namespace string) *api.BackendAuthPolicy {
	if authToken == nil {
		return nil
	}

	switch authToken.Kind {
	case v1alpha1.SecretRef:
		if authToken.SecretRef == nil {
			return nil
		}

		// Get secret using the SecretIndex
		secret, err := pluginutils.GetSecretIr(secrets, krtctx, authToken.SecretRef.Name, namespace)
		if err != nil {
			// Return nil auth policy if secret not found - this will be handled upstream
			// TODO(npolshak): Add backend status errors https://github.com/kgateway-dev/kgateway/issues/11966
			return nil
		}

		// Extract the authorization key from the secret data
		authKey := ""
		if authValue, exists := getSecretValue(secret, "Authorization"); exists {
			// Strip the "Bearer " prefix if present, as it will be added by the provider
			authValue = strings.TrimSpace(authValue)
			authKey = strings.TrimSpace(strings.TrimPrefix(authValue, "Bearer "))
		}

		if authKey == "" {
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
func buildMCPIr(krtctx krt.HandlerContext, be *v1alpha1.Backend, services krt.Collection[*corev1.Service], namespaces krt.Collection[krtcollections.NamespaceMetadata]) (*MCPIr, error) {
	if be.Spec.MCP == nil {
		return nil, fmt.Errorf("mcp backend spec must not be nil for MCP backend type")
	}

	var mcpTargets []*api.MCPTarget
	var backends []*api.Backend
	serviceEndpoints := make(map[string]*ServiceEndpoint)

	// Process each target selector
	for _, targetSelector := range be.Spec.MCP.Targets {
		// Handle static targets
		if targetSelector.StaticTarget != nil {
			staticBackendRef := be.Namespace + "/" + targetSelector.StaticTarget.Name
			staticBackend := &api.Backend{
				Name: staticBackendRef,
				Kind: &api.Backend_Static{
					Static: &api.StaticBackend{
						Host: targetSelector.StaticTarget.Host,
						Port: targetSelector.StaticTarget.Port,
					},
				},
			}
			backends = append(backends, staticBackend)

			mcpTarget := &api.MCPTarget{
				Name: targetSelector.StaticTarget.Name,
				Backend: &api.BackendReference{
					Kind: &api.BackendReference_Backend{
						Backend: staticBackendRef,
					},
					Port: uint32(targetSelector.StaticTarget.Port),
				},
			}

			// Convert protocol if specified
			switch targetSelector.StaticTarget.Protocol {
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
				Host:      targetSelector.StaticTarget.Host,
				Port:      targetSelector.StaticTarget.Port,
				Namespace: be.Namespace,
			}
		}

		// Handle service selectors
		if targetSelector.Selectors != nil {
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
			if targetSelector.Selectors.ServiceSelector != nil {
				serviceSelector, err := metav1.LabelSelectorAsSelector(targetSelector.Selectors.ServiceSelector)
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
			if targetSelector.Selectors.NamespaceSelector != nil {
				namespaceSelector, err := metav1.LabelSelectorAsSelector(targetSelector.Selectors.NamespaceSelector)
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
					if port.AppProtocol == nil || *port.AppProtocol != mcpProtocol {
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
							Port: uint32(port.Port),
						},
						// TODO: Determine protocol from service annotations or other metadata
						// For now, default to undefined protocol
						Protocol: api.MCPTarget_UNDEFINED,
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

// getSecretValue extracts a value from a Kubernetes secret, handling both Data and StringData fields.
// It prioritizes StringData over Data if both are present.
func getSecretValue(secret *ir.Secret, key string) (string, bool) {
	if value, exists := secret.Data[key]; exists && utf8.Valid(value) {
		return strings.TrimSpace(string(value)), true
	}

	return "", false
}

func buildBedrockAuthPolicy(krtctx krt.HandlerContext, region string, auth *v1alpha1.AwsAuth, secrets *krtcollections.SecretIndex, namespace string) (*api.BackendAuthPolicy, error) {
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
		secret, err := pluginutils.GetSecretIr(secrets, krtctx, auth.SecretRef.Name, namespace)
		if err != nil {
			// Return nil auth policy if secret not found - this will be handled upstream
			// TODO(npolshak): Add backend status errors https://github.com/kgateway-dev/kgateway/issues/11966
			return nil, err
		}

		var accessKeyId, secretAccessKey string
		var sessionToken *string

		// Extract access key
		if value, exists := getSecretValue(secret, wellknown.AccessKey); !exists {
			errs = append(errs, errors.New("accessKey is missing or not a valid string"))
		} else {
			accessKeyId = value
		}

		// Extract secret key
		if value, exists := getSecretValue(secret, wellknown.SecretKey); !exists {
			errs = append(errs, errors.New("secretKey is missing or not a valid string"))
		} else {
			secretAccessKey = value
		}

		// Extract session token (optional)
		if value, exists := getSecretValue(secret, wellknown.SessionToken); exists {
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
