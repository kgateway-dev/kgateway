package ai

import (
	"fmt"
	"log/slog"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	aiutils "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

const (
	tlsPort = 443

	// well-known provider default hosts
	OpenAIHost    = "api.openai.com"
	GeminiHost    = "generativelanguage.googleapis.com"
	AnthropicHost = "api.anthropic.com"
)

func tlsMatch() *structpb.Struct {
	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"tls": structpb.NewStringValue("true"),
		},
	}
}

func ProcessAIBackend(in *v1alpha1.AIBackend, aiSecret *ir.Secret, multiSecrets map[string]*ir.Secret, out *envoyclusterv3.Cluster) error {
	if in == nil {
		return nil
	}

	if err := buildModelCluster(in, aiSecret, multiSecrets, out); err != nil {
		return err
	}

	return nil
}

// buildModelCluster builds a cluster for the given AI backend.
// This function is used by the `ProcessBackend` function to build the cluster for the AI backend.
// It is ALSO used by `ProcessRoute` to create the cluster in the event of backup models being used
// and fallbacks being required.
func buildModelCluster(aiUs *v1alpha1.AIBackend, aiSecret *ir.Secret, multiSecrets map[string]*ir.Secret, out *envoyclusterv3.Cluster) error {
	// set the type to strict dns to support mutli pool backends
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
		Type: envoyclusterv3.Cluster_STRICT_DNS,
	}

	// We are reliant on https://github.com/envoyproxy/envoy/pull/34154 to merge
	// before we can do OutlierDetection on 429s here
	// out.OutlierDetection = getOutlierDetectionConfig(aiUs)

	var prioritized []*envoyendpointv3.LocalityLbEndpoints
	var err error

	if aiUs.MultiPool != nil {
		epByType := map[string]struct{}{}
		prioritized = make([]*envoyendpointv3.LocalityLbEndpoints, 0, len(aiUs.MultiPool.Priorities))
		for idx, pool := range aiUs.MultiPool.Priorities {
			eps := make([]*envoyendpointv3.LbEndpoint, 0, len(pool.Pool))
			for jdx, ep := range pool.Pool {
				var result *envoyendpointv3.LbEndpoint
				var err error
				epByType[fmt.Sprintf("%T", ep)] = struct{}{}
				if ep.Provider.OpenAI != nil {
					var secretForMultiPool *ir.Secret
					if ep.Provider.OpenAI.AuthToken.Kind == v1alpha1.SecretRef {
						secretRef := ep.Provider.OpenAI.AuthToken.SecretRef
						secretForMultiPool = multiSecrets[GetMultiPoolSecretKey(idx, jdx, secretRef.Name)]
					}
					result, err = buildOpenAIEndpoint(ep.Provider.OpenAI, ep.HostOverride, secretForMultiPool)
				} else if ep.Provider.Anthropic != nil {
					var secretForMultiPool *ir.Secret
					if ep.Provider.Anthropic.AuthToken.Kind == v1alpha1.SecretRef {
						secretRef := ep.Provider.Anthropic.AuthToken.SecretRef
						secretForMultiPool = multiSecrets[GetMultiPoolSecretKey(idx, jdx, secretRef.Name)]
					}
					result, err = buildAnthropicEndpoint(ep.Provider.Anthropic, ep.HostOverride, secretForMultiPool)
				} else if ep.Provider.AzureOpenAI != nil {
					var secretForMultiPool *ir.Secret
					if ep.Provider.AzureOpenAI.AuthToken.Kind == v1alpha1.SecretRef {
						secretRef := ep.Provider.AzureOpenAI.AuthToken.SecretRef
						secretForMultiPool = multiSecrets[GetMultiPoolSecretKey(idx, jdx, secretRef.Name)]
					}
					result, err = buildAzureOpenAIEndpoint(ep.Provider.AzureOpenAI, ep.HostOverride, secretForMultiPool)
				} else if ep.Provider.Gemini != nil {
					var secretForMultiPool *ir.Secret
					if ep.Provider.Gemini.AuthToken.Kind == v1alpha1.SecretRef {
						secretRef := ep.Provider.Gemini.AuthToken.SecretRef
						secretForMultiPool = multiSecrets[GetMultiPoolSecretKey(idx, jdx, secretRef.Name)]
					}
					result, err = buildGeminiEndpoint(ep.Provider.Gemini, ep.HostOverride, secretForMultiPool)
				} else if ep.Provider.VertexAI != nil {
					var secretForMultiPool *ir.Secret
					if ep.Provider.VertexAI.AuthToken.Kind == v1alpha1.SecretRef {
						secretRef := ep.Provider.VertexAI.AuthToken.SecretRef
						secretForMultiPool = multiSecrets[GetMultiPoolSecretKey(idx, jdx, secretRef.Name)]
					}
					result, err = buildVertexAIEndpoint(ep.Provider.VertexAI, ep.HostOverride, secretForMultiPool)
				} else if ep.Provider.Bedrock != nil {
					// currently only supported in agentgateway
					slog.Error("bedrock on the AI backend are not supported yet, switch to agentgateway class")
				}
				if err != nil {
					return err
				}
				eps = append(eps, result)
			}
			priority := idx
			prioritized = append(prioritized, &envoyendpointv3.LocalityLbEndpoints{
				Priority:    uint32(priority),
				LbEndpoints: eps,
			})
		}
		if len(epByType) > 1 {
			return fmt.Errorf("multi backend pools must all be of the same type, got %v", epByType)
		}
	} else if aiUs.LLM != nil {
		prioritized, err = buildLLMEndpoint(aiUs, aiSecret)
		if err != nil {
			return err
		}
	}

	// TODO: ssl validation https://github.com/kgateway-dev/kgateway/issues/10719
	// attempt to match tls, the default match is always plaintext
	tlsCtx := &envoytlsv3.UpstreamTlsContext{
		CommonTlsContext: &envoytlsv3.CommonTlsContext{},
		AutoHostSni:      true,
	}
	tlsCtxAny, err := utils.MessageToAny(tlsCtx)
	if err != nil {
		return err
	}
	out.TransportSocketMatches = append(out.GetTransportSocketMatches(), []*envoyclusterv3.Cluster_TransportSocketMatch{
		{
			Name: "tls",
			TransportSocket: &envoycorev3.TransportSocket{
				Name: wellknown.TransportSocketTls,
				ConfigType: &envoycorev3.TransportSocket_TypedConfig{
					TypedConfig: tlsCtxAny,
				},
			},
			Match: tlsMatch(),
		},
		{
			Name: "plaintext",
			TransportSocket: &envoycorev3.TransportSocket{
				Name: wellknown.TransportSocketRawBuffer,
				ConfigType: &envoycorev3.TransportSocket_TypedConfig{
					TypedConfig: &anypb.Any{
						TypeUrl: "type.googleapis.com/envoy.extensions.transport_sockets.raw_buffer.v3.RawBuffer",
					},
				},
			},
			Match: &structpb.Struct{},
		},
	}...)
	out.LoadAssignment = &envoyendpointv3.ClusterLoadAssignment{
		ClusterName: out.GetName(),
		Endpoints:   prioritized,
	}

	return nil
}

func buildLLMEndpoint(aiUs *v1alpha1.AIBackend, aiSecrets *ir.Secret) ([]*envoyendpointv3.LocalityLbEndpoints, error) {
	var prioritized []*envoyendpointv3.LocalityLbEndpoints
	provider := aiUs.LLM.Provider
	if provider.OpenAI != nil {
		host, err := buildOpenAIEndpoint(provider.OpenAI, aiUs.LLM.HostOverride, aiSecrets)
		if err != nil {
			return nil, err
		}
		prioritized = []*envoyendpointv3.LocalityLbEndpoints{
			{LbEndpoints: []*envoyendpointv3.LbEndpoint{host}},
		}
	} else if provider.Anthropic != nil {
		host, err := buildAnthropicEndpoint(provider.Anthropic, aiUs.LLM.HostOverride, aiSecrets)
		if err != nil {
			return nil, err
		}
		prioritized = []*envoyendpointv3.LocalityLbEndpoints{
			{LbEndpoints: []*envoyendpointv3.LbEndpoint{host}},
		}
	} else if provider.AzureOpenAI != nil {
		host, err := buildAzureOpenAIEndpoint(provider.AzureOpenAI, aiUs.LLM.HostOverride, aiSecrets)
		if err != nil {
			return nil, err
		}
		prioritized = []*envoyendpointv3.LocalityLbEndpoints{
			{LbEndpoints: []*envoyendpointv3.LbEndpoint{host}},
		}
	} else if provider.Gemini != nil {
		host, err := buildGeminiEndpoint(provider.Gemini, aiUs.LLM.HostOverride, aiSecrets)
		if err != nil {
			return nil, err
		}
		prioritized = []*envoyendpointv3.LocalityLbEndpoints{
			{LbEndpoints: []*envoyendpointv3.LbEndpoint{host}},
		}
	} else if provider.VertexAI != nil {
		host, err := buildVertexAIEndpoint(provider.VertexAI, aiUs.LLM.HostOverride, aiSecrets)
		if err != nil {
			return nil, err
		}
		prioritized = []*envoyendpointv3.LocalityLbEndpoints{
			{LbEndpoints: []*envoyendpointv3.LbEndpoint{host}},
		}
	}
	return prioritized, nil
}

func buildOpenAIEndpoint(data *v1alpha1.OpenAIConfig, hostOverride *v1alpha1.Host, aiSecrets *ir.Secret) (*envoyendpointv3.LbEndpoint, error) {
	token, err := aiutils.GetAuthToken(data.AuthToken, aiSecrets)
	if err != nil {
		return nil, err
	}
	model := ""
	if data.Model != nil {
		model = *data.Model
	}
	return buildLocalityLbEndpoint(
		OpenAIHost,
		tlsPort,
		hostOverride,
		buildEndpointMeta(token, model, nil),
	), nil
}

func buildAnthropicEndpoint(data *v1alpha1.AnthropicConfig, hostOverride *v1alpha1.Host, aiSecrets *ir.Secret) (*envoyendpointv3.LbEndpoint, error) {
	token, err := aiutils.GetAuthToken(data.AuthToken, aiSecrets)
	if err != nil {
		return nil, err
	}
	model := ""
	if data.Model != nil {
		model = *data.Model
	}
	return buildLocalityLbEndpoint(
		AnthropicHost,
		tlsPort,
		hostOverride,
		buildEndpointMeta(token, model, nil),
	), nil
}

func buildAzureOpenAIEndpoint(data *v1alpha1.AzureOpenAIConfig, hostOverride *v1alpha1.Host, aiSecrets *ir.Secret) (*envoyendpointv3.LbEndpoint, error) {
	token, err := aiutils.GetAuthToken(data.AuthToken, aiSecrets)
	if err != nil {
		return nil, err
	}
	return buildLocalityLbEndpoint(
		data.Endpoint,
		tlsPort,
		hostOverride,
		buildEndpointMeta(token, data.DeploymentName, map[string]string{"api_version": data.ApiVersion}),
	), nil
}

func buildGeminiEndpoint(data *v1alpha1.GeminiConfig, hostOverride *v1alpha1.Host, aiSecrets *ir.Secret) (*envoyendpointv3.LbEndpoint, error) {
	token, err := aiutils.GetAuthToken(data.AuthToken, aiSecrets)
	if err != nil {
		return nil, err
	}
	return buildLocalityLbEndpoint(
		GeminiHost,
		tlsPort,
		hostOverride,
		buildEndpointMeta(token, data.Model, map[string]string{"api_version": data.ApiVersion}),
	), nil
}

func buildVertexAIEndpoint(data *v1alpha1.VertexAIConfig, hostOverride *v1alpha1.Host, aiSecrets *ir.Secret) (*envoyendpointv3.LbEndpoint, error) {
	token, err := aiutils.GetAuthToken(data.AuthToken, aiSecrets)
	if err != nil {
		return nil, err
	}
	var publisher string
	switch data.Publisher {
	case v1alpha1.GOOGLE:
		publisher = "google"
	default:
		// TODO(npolshak): add support for other publishers
		slog.Warn("unsupported Vertex AI publisher, defaulting to Google", "publisher", string(data.Publisher))
		publisher = "google"
	}
	return buildLocalityLbEndpoint(
		fmt.Sprintf("%s-aiplatform.googleapis.com", data.Location),
		tlsPort,
		hostOverride,
		buildEndpointMeta(token, data.Model, map[string]string{"api_version": data.ApiVersion, "location": data.Location, "project": data.ProjectId, "publisher": publisher}),
	), nil
}

func buildLocalityLbEndpoint(
	host string,
	port int32,
	hostOverride *v1alpha1.Host,
	metadata *envoycorev3.Metadata,
) *envoyendpointv3.LbEndpoint {
	if hostOverride != nil {
		if hostOverride.Host != "" {
			host = hostOverride.Host
		}
		if hostOverride.Port != 0 {
			port = int32(hostOverride.Port)
		}
	}
	if port == tlsPort {
		// Used for transport socket matching
		metadata.GetFilterMetadata()["envoy.transport_socket_match"] = tlsMatch()
	}

	return &envoyendpointv3.LbEndpoint{
		Metadata: metadata,
		HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
			Endpoint: &envoyendpointv3.Endpoint{
				Hostname: host,
				Address: &envoycorev3.Address{
					Address: &envoycorev3.Address_SocketAddress{
						SocketAddress: &envoycorev3.SocketAddress{
							Protocol: envoycorev3.SocketAddress_TCP,
							Address:  host,
							PortSpecifier: &envoycorev3.SocketAddress_PortValue{
								PortValue: uint32(port),
							},
						},
					},
				},
			},
		},
	}
}

// `buildEndpointMeta` builds the metadata for the endpoint.
// This metadata is used by the post routing transformation filter to modify the request body.
func buildEndpointMeta(token, model string, additionalFields map[string]string) *envoycorev3.Metadata {
	fields := map[string]*structpb.Value{
		"auth_token": structpb.NewStringValue(token),
	}
	if model != "" {
		fields["model"] = structpb.NewStringValue(model)
	}
	for k, v := range additionalFields {
		fields[k] = structpb.NewStringValue(v)
	}
	return &envoycorev3.Metadata{
		FilterMetadata: map[string]*structpb.Struct{
			"io.solo.transformation": {
				Fields: fields,
			},
		},
	}
}

func createTransformationTemplate(aiBackend *v1alpha1.AIBackend) *envoytransformation.TransformationTemplate {
	// Setup initial transformation template. This may be modified by further
	transformationTemplate := &envoytransformation.TransformationTemplate{
		// We will add the auth token later
		Headers: map[string]*envoytransformation.InjaTemplate{},
	}

	var headerName, prefix, path string
	var bodyTransformation *envoytransformation.TransformationTemplate_MergeJsonKeys
	if aiBackend.LLM != nil {
		headerName, prefix, path, bodyTransformation = getTransformation(aiBackend.LLM)
	} else if aiBackend.MultiPool != nil {
		// We already know that all the backends are the same type so we can just take the first one
		llmMultiPool := aiBackend.MultiPool.Priorities[0].Pool[0]
		headerName, prefix, path, bodyTransformation = getTransformation(&llmMultiPool)
	}
	transformationTemplate.GetHeaders()[headerName] = &envoytransformation.InjaTemplate{
		Text: prefix + `{% if host_metadata("auth_token") != "" %}{{host_metadata("auth_token")}}{% else %}{{dynamic_metadata("auth_token","ai.kgateway.io")}}{% endif %}`,
	}
	transformationTemplate.GetHeaders()[":path"] = &envoytransformation.InjaTemplate{
		Text: path,
	}
	transformationTemplate.BodyTransformation = bodyTransformation
	return transformationTemplate
}

func getTransformation(llm *v1alpha1.LLMProvider) (string, string, string, *envoytransformation.TransformationTemplate_MergeJsonKeys) {
	headerName := "Authorization"
	var prefix, path string
	var bodyTransformation *envoytransformation.TransformationTemplate_MergeJsonKeys
	provider := llm.Provider
	if provider.OpenAI != nil {
		prefix = "Bearer "
		path = "/v1/chat/completions"
		bodyTransformation = defaultBodyTransformation()
	} else if provider.Anthropic != nil {
		headerName = "x-api-key"
		path = "/v1/messages"
		bodyTransformation = defaultBodyTransformation()
	} else if provider.AzureOpenAI != nil {
		headerName = "api-key"
		path = `/openai/deployments/{{ host_metadata("model") }}/chat/completions?api-version={{ host_metadata("api_version" )}}`
	} else if provider.Gemini != nil {
		headerName = "key"
		path = getGeminiPath()
	} else if provider.VertexAI != nil {
		prefix = "Bearer "
		var modelPath string
		modelCall := provider.VertexAI.ModelPath
		if modelCall == nil {
			switch provider.VertexAI.Publisher {
			case v1alpha1.GOOGLE:
				modelPath = getVertexAIGeminiModelPath()
			default:
				// TODO(npolshak): add support for other publishers
				slog.Warn("unsupported Vertex AI publisher, defaulting to Google", "publisher", string(provider.VertexAI.Publisher))
				modelPath = getVertexAIGeminiModelPath()
			}
		} else {
			// Use user provided model path
			modelPath = fmt.Sprintf(`models/{{host_metadata("model")}}:%s`, *modelCall)
		}
		// https://${LOCATION}-aiplatform.googleapis.com/${VERSION}/projects/${PROJECT_ID}/locations/${LOCATION}/publishers/${PUBLISHER}/models/${MODEL}:{generateContent|streamGenerateContent}
		path = fmt.Sprintf(`/{{host_metadata("api_version")}}/projects/{{host_metadata("project")}}/locations/{{host_metadata("location")}}/publishers/{{host_metadata("publisher")}}/%s`, modelPath)
	}
	if llm.PathOverride != nil {
		path = *llm.PathOverride.FullPath
	}
	if llm.AuthHeaderOverride != nil {
		if llm.AuthHeaderOverride.HeaderName != nil {
			headerName = *llm.AuthHeaderOverride.HeaderName
		}
		if llm.AuthHeaderOverride.Prefix != nil {
			prefix = *llm.AuthHeaderOverride.Prefix
		}
	}

	return headerName, prefix, path, bodyTransformation
}

func getGeminiPath() string {
	return `/{{host_metadata("api_version")}}/models/{{host_metadata("model")}}:{% if dynamic_metadata("route_type") == "CHAT_STREAMING" %}streamGenerateContent?key={{host_metadata("auth_token")}}&alt=sse{% else %}generateContent?key={{host_metadata("auth_token")}}{% endif %}`
}

func getVertexAIGeminiModelPath() string {
	return `models/{{host_metadata("model")}}:{% if dynamic_metadata("route_type") == "CHAT_STREAMING" %}streamGenerateContent?alt=sse{% else %}generateContent{% endif %}`
}

func defaultBodyTransformation() *envoytransformation.TransformationTemplate_MergeJsonKeys {
	return &envoytransformation.TransformationTemplate_MergeJsonKeys{
		MergeJsonKeys: &envoytransformation.MergeJsonKeys{
			JsonKeys: map[string]*envoytransformation.MergeJsonKeys_OverridableTemplate{
				"model": {
					Tmpl: &envoytransformation.InjaTemplate{
						// Merge the model into the body
						Text: `{% if host_metadata("model") != "" %}"{{host_metadata("model")}}"{% else %}"{{model}}"{% endif %}`,
					},
				},
			},
		},
	}
}

func GetMultiPoolSecretKey(priorityIdx, poolIdx int, secretName string) string {
	return fmt.Sprintf("%d-%d-%s", priorityIdx, poolIdx, secretName)
}
