package upstream

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/rotisserie/eris"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

const (
	tlsPort = 443
)

func processAIUpstream(ctx context.Context, in *v1alpha1.AIUpstream, ir *UpstreamIr, out *envoy_config_cluster_v3.Cluster) error {
	if in == nil {
		return nil
	}

	if err := buildModelCluster(ctx, in, ir, out); err != nil {
		return err
	}

	return nil
}

// buildModelCluster builds a cluster for the given AI upstream.
// This function is used by the `ProcessUpstream` function to build the cluster for the AI upstream.
// It is ALSO used by `ProcessRoute` to create the cluster in the event of backup models being used
// and fallbacks being required.
func buildModelCluster(ctx context.Context, aiUs *v1alpha1.AIUpstream, ir *UpstreamIr, out *envoy_config_cluster_v3.Cluster) error {
	// set the type to strict dns
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_STRICT_DNS,
	}

	// fix issue where ipv6 addr cannot bind
	out.DnsLookupFamily = envoy_config_cluster_v3.Cluster_V4_ONLY

	// We are reliant on https://github.com/envoyproxy/envoy/pull/34154 to merge
	// before we can do OutlierDetection on 429s here
	// out.OutlierDetection = getOutlierDetectionConfig(aiUs)

	var prioritized []*envoy_config_endpoint_v3.LocalityLbEndpoints
	var matches []*envoy_config_cluster_v3.Cluster_TransportSocketMatch
	var err error

	if aiUs.MultiPool != nil {
		epByType := map[string]struct{}{}
		tsmByHost := make(map[string]*envoy_config_cluster_v3.Cluster_TransportSocketMatch)
		prioritized = make([]*envoy_config_endpoint_v3.LocalityLbEndpoints, 0, len(aiUs.MultiPool.Priorities))
		for idx, pool := range aiUs.MultiPool.Priorities {
			eps := make([]*envoy_config_endpoint_v3.LbEndpoint, 0, len(pool.Pool))
			for _, ep := range pool.Pool {
				var result *envoy_config_endpoint_v3.LbEndpoint
				var tlsContext *envoy_tls_v3.UpstreamTlsContext
				var err error
				epByType[fmt.Sprintf("%T", ep)] = struct{}{}
				if ep.OpenAI != nil {
					result, tlsContext, err = buildOpenAIEndpoint(ep.OpenAI, ir)
				} else if ep.Anthropic != nil {
					result, tlsContext, err = buildAnthropicEndpoint(ep.Anthropic, ir)
				} else if ep.AzureOpenAI != nil {
					result, tlsContext, err = buildAzureOpenAIEndpoint(ep.AzureOpenAI, ir)
				} else if ep.Gemini != nil {
					result, tlsContext, err = buildGeminiEndpoint(ep.Gemini, ir)
				} else if ep.VertexAI != nil {
					result, tlsContext, err = buildVertexAIEndpoint(ctx, ep.VertexAI, ir)
				}
				if err != nil {
					return err
				}
				eps = append(eps, result)
				if tlsContext == nil {
					continue
				}
				if _, ok := tsmByHost[tlsContext.GetSni()]; !ok {
					tsm, err := buildTsm(tlsContext)
					if err != nil {
						return err
					}
					tsmByHost[tlsContext.GetSni()] = tsm
				}
			}
			priority := idx
			prioritized = append(prioritized, &envoy_config_endpoint_v3.LocalityLbEndpoints{
				Priority:    uint32(priority),
				LbEndpoints: eps,
			})
		}
		if len(epByType) > 1 {
			return eris.Errorf("multi backend pools must all be of the same type, got %v", epByType)
		}
		slice := slices.Collect(maps.Values(tsmByHost))
		slices.SortStableFunc(slice, func(a, b *envoy_config_cluster_v3.Cluster_TransportSocketMatch) int {
			return strings.Compare(a.GetName(), b.GetName())
		})
		out.TransportSocketMatches = append(out.GetTransportSocketMatches(), slice...)
	} else if aiUs.LLM != nil {
		matches, prioritized, err = buildLLMEndpoint(ctx, aiUs, ir)
		if err != nil {
			return err
		}
		out.TransportSocketMatches = matches
	}
	// Default match on plaintext if nothing else is added
	out.TransportSocketMatches = append(out.GetTransportSocketMatches(), &envoy_config_cluster_v3.Cluster_TransportSocketMatch{
		Name: "plaintext",
		TransportSocket: &envoy_config_core_v3.TransportSocket{
			Name: wellknown.TransportSocketRawBuffer,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
				TypedConfig: &anypb.Any{
					TypeUrl: "type.googleapis.com/envoy.extensions.transport_sockets.raw_buffer.v3.RawBuffer",
				},
			},
		},
		Match: &structpb.Struct{},
	})
	out.LoadAssignment = &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: out.GetName(),
		Endpoints:   prioritized,
	}

	return nil
}

func buildLLMEndpoint(ctx context.Context, aiUs *v1alpha1.AIUpstream, ir *UpstreamIr) ([]*envoy_config_cluster_v3.Cluster_TransportSocketMatch, []*envoy_config_endpoint_v3.LocalityLbEndpoints, error) {
	var tsms []*envoy_config_cluster_v3.Cluster_TransportSocketMatch
	var prioritized []*envoy_config_endpoint_v3.LocalityLbEndpoints
	if aiUs.LLM.OpenAI != nil {
		host, tlsContext, err := buildOpenAIEndpoint(aiUs.LLM.OpenAI, ir)
		if err != nil {
			return nil, nil, err
		}
		prioritized = []*envoy_config_endpoint_v3.LocalityLbEndpoints{
			{LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{host}},
		}
		if tlsContext != nil {
			tsm, err := buildTsm(tlsContext)
			if err != nil {
				return nil, nil, err
			}
			tsms = append(tsms, tsm)
		}
	} else if aiUs.LLM.Anthropic != nil {
		host, tlsContext, err := buildAnthropicEndpoint(aiUs.LLM.Anthropic, ir)
		if err != nil {
			return nil, nil, err
		}
		prioritized = []*envoy_config_endpoint_v3.LocalityLbEndpoints{
			{LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{host}},
		}
		if tlsContext != nil {
			tsm, err := buildTsm(tlsContext)
			if err != nil {
				return nil, nil, err
			}
			tsms = append(tsms, tsm)
		}
	} else if aiUs.LLM.AzureOpenAI != nil {
		host, tlsContext, err := buildAzureOpenAIEndpoint(aiUs.LLM.AzureOpenAI, ir)
		if err != nil {
			return nil, nil, err
		}
		prioritized = []*envoy_config_endpoint_v3.LocalityLbEndpoints{
			{LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{host}},
		}
		if tlsContext != nil {
			tsm, err := buildTsm(tlsContext)
			if err != nil {
				return nil, nil, err
			}
			tsms = append(tsms, tsm)
		}
	} else if aiUs.LLM.Gemini != nil {
		host, tlsContext, err := buildGeminiEndpoint(aiUs.LLM.Gemini, ir)
		if err != nil {
			return nil, nil, err
		}
		prioritized = []*envoy_config_endpoint_v3.LocalityLbEndpoints{
			{LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{host}},
		}
		if tlsContext != nil {
			tsm, err := buildTsm(tlsContext)
			if err != nil {
				return nil, nil, err
			}
			tsms = append(tsms, tsm)
		}
	} else if aiUs.LLM.VertexAI != nil {
		host, tlsContext, err := buildVertexAIEndpoint(ctx, aiUs.LLM.VertexAI, ir)
		if err != nil {
			return nil, nil, err
		}
		prioritized = []*envoy_config_endpoint_v3.LocalityLbEndpoints{
			{LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{host}},
		}
		if tlsContext != nil {
			tsm, err := buildTsm(tlsContext)
			if err != nil {
				return nil, nil, err
			}
			tsms = append(tsms, tsm)
		}
	}
	return tsms, prioritized, nil
}

// Build a TransoprtSocketMatch for the given UpstreamTlsContext.
func buildTsm(tlsContext *envoy_tls_v3.UpstreamTlsContext) (*envoy_config_cluster_v3.Cluster_TransportSocketMatch, error) {
	typedConfig, err := utils.MessageToAny(tlsContext)
	if err != nil {
		return nil, err
	}
	return &envoy_config_cluster_v3.Cluster_TransportSocketMatch{
		Name: "tls_" + tlsContext.GetSni(),
		Match: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"tls": structpb.NewStringValue(tlsContext.GetSni()),
			},
		},
		TransportSocket: &envoy_config_core_v3.TransportSocket{
			Name:       wellknown.TransportSocketTls,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
		},
	}, nil
}

func buildOpenAIEndpoint(data *v1alpha1.OpenAIConfig, ir *UpstreamIr) (*envoy_config_endpoint_v3.LbEndpoint, *envoy_tls_v3.UpstreamTlsContext, error) {
	token, err := getAuthToken(data.AuthToken, ir)
	if err != nil {
		return nil, nil, err
	}
	model := ""
	if data.Model != nil {
		model = *data.Model
	}
	ep, host := buildLocalityLbEndpoint(
		"api.openai.com",
		tlsPort,
		data.CustomHost,
		buildEndpointMeta(token, model, nil),
	)
	return ep, host, nil
}
func buildAnthropicEndpoint(data *v1alpha1.AnthropicConfig, ir *UpstreamIr) (*envoy_config_endpoint_v3.LbEndpoint, *envoy_tls_v3.UpstreamTlsContext, error) {
	token, err := getAuthToken(data.AuthToken, ir)
	if err != nil {
		return nil, nil, err
	}
	model := ""
	if data.Model != nil {
		model = *data.Model
	}
	ep, host := buildLocalityLbEndpoint(
		"api.anthropic.com",
		tlsPort,
		data.CustomHost,
		buildEndpointMeta(token, model, nil),
	)
	return ep, host, nil
}
func buildAzureOpenAIEndpoint(data *v1alpha1.AzureOpenAIConfig, ir *UpstreamIr) (*envoy_config_endpoint_v3.LbEndpoint, *envoy_tls_v3.UpstreamTlsContext, error) {
	token, err := getAuthToken(data.AuthToken, ir)
	if err != nil {
		return nil, nil, err
	}
	ep, host := buildLocalityLbEndpoint(
		data.Endpoint,
		tlsPort,
		nil,
		buildEndpointMeta(token, data.DeploymentName, map[string]string{"api_version": data.ApiVersion}),
	)
	return ep, host, nil
}
func buildGeminiEndpoint(data *v1alpha1.GeminiConfig, ir *UpstreamIr) (*envoy_config_endpoint_v3.LbEndpoint, *envoy_tls_v3.UpstreamTlsContext, error) {
	token, err := getAuthToken(data.AuthToken, ir)
	if err != nil {
		return nil, nil, err
	}
	ep, host := buildLocalityLbEndpoint(
		"generativelanguage.googleapis.com",
		tlsPort,
		nil,
		buildEndpointMeta(token, data.Model, map[string]string{"api_version": data.ApiVersion}),
	)
	return ep, host, nil
}
func buildVertexAIEndpoint(ctx context.Context, data *v1alpha1.VertexAIConfig, ir *UpstreamIr) (*envoy_config_endpoint_v3.LbEndpoint, *envoy_tls_v3.UpstreamTlsContext, error) {
	token, err := getAuthToken(data.AuthToken, ir)
	if err != nil {
		return nil, nil, err
	}
	var publisher string
	switch data.Publisher {
	case v1alpha1.GOOGLE:
		publisher = "google"
	default:
		// TODO(npolshak): add support for other publishers
		contextutils.LoggerFrom(ctx).Warnf("unsupported Vertex AI publisher: %v. Defaulting to Google.", data.Publisher)
		publisher = "google"
	}
	ep, host := buildLocalityLbEndpoint(
		fmt.Sprintf("%s-aiplatform.googleapis.com", data.Location),
		tlsPort,
		nil,
		buildEndpointMeta(token, data.Model, map[string]string{"api_version": data.ApiVersion, "location": data.Location, "project": data.ProjectId, "publisher": publisher}),
	)
	return ep, host, nil
}
func buildLocalityLbEndpoint(
	host string,
	port int32,
	customHost *v1alpha1.Host,
	metadata *envoy_config_core_v3.Metadata,
) (*envoy_config_endpoint_v3.LbEndpoint, *envoy_tls_v3.UpstreamTlsContext) {
	if customHost != nil {
		if customHost.Host != "" {
			host = customHost.Host
		}
		if customHost.Port != 0 {
			port = int32(customHost.Port)
		}
	}
	var tlsContext *envoy_tls_v3.UpstreamTlsContext
	if port == tlsPort {
		// Used for transport socket matching
		metadata.GetFilterMetadata()["envoy.transport_socket_match"] = &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"tls": structpb.NewStringValue(host),
			},
		}
		tlsContext = &envoy_tls_v3.UpstreamTlsContext{
			CommonTlsContext: &envoy_tls_v3.CommonTlsContext{},
			Sni:              host,
		}
	}
	return &envoy_config_endpoint_v3.LbEndpoint{
		Metadata: metadata,
		HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
			Endpoint: &envoy_config_endpoint_v3.Endpoint{
				Hostname: host,
				Address: &envoy_config_core_v3.Address{
					Address: &envoy_config_core_v3.Address_SocketAddress{
						SocketAddress: &envoy_config_core_v3.SocketAddress{
							Protocol: envoy_config_core_v3.SocketAddress_TCP,
							Address:  host,
							PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
								PortValue: uint32(port),
							},
						},
					},
				},
			},
		},
	}, tlsContext
}
func getAuthToken(in v1alpha1.SingleAuthToken, ir *UpstreamIr) (token string, err error) {
	switch in.Kind {
	case v1alpha1.Inline:
		if in.Inline == nil {
			return "", errors.New("inline auth token must be set if kind is type Inline.")
		}
		token = *in.Inline
	case v1alpha1.SecretRef:
		secret, err := deriveHeaderSecret(ir.AISecret)
		if err != nil {
			return "", err
		}
		token = getTokenFromHeaderSecret(secret)
	}
	return token, err
}

// `buildEndpointMeta` builds the metadata for the endpoint.
// This metadata is used by the post routing transformation filter to modify the request body.
func buildEndpointMeta(token, model string, additionalFields map[string]string) *envoy_config_core_v3.Metadata {
	fields := map[string]*structpb.Value{
		"auth_token": structpb.NewStringValue(token),
	}
	if model != "" {
		fields["model"] = structpb.NewStringValue(model)
	}
	for k, v := range additionalFields {
		fields[k] = structpb.NewStringValue(v)
	}
	return &envoy_config_core_v3.Metadata{
		FilterMetadata: map[string]*structpb.Struct{
			"io.solo.transformation": {
				Fields: fields,
			},
		},
	}
}

const (
	AuthKey = "Authorization"
)

type headerSecretDerivation struct {
	authorization string
}

// deriveHeaderSecret from ingest if we are using a kubernetes secretref
// Named returns with the derived string contents or an error due to retrieval or format.
func deriveHeaderSecret(aiSecrets *ir.Secret) (headerSecretDerivation, error) {
	var errs []error
	derived := headerSecretDerivation{
		authorization: string(aiSecrets.Data[AuthKey]),
	}
	if derived.authorization == "" || !utf8.Valid([]byte(derived.authorization)) {
		// err is nil here but this is still safe
		errs = append(errs, errors.New("access_key is not a valid string"))
	}
	return derived, errors.Join(errs...)
}

// `getTokenFromHeaderSecret` retrieves the auth token from the secret reference.
// Currently, this function will return an error if there are more than one header in the secret
// as we do not know which one to select.
// In addition, this function will strip the "Bearer " prefix from the token as it will get conditionally
// added later depending on the provider.
func getTokenFromHeaderSecret(secret headerSecretDerivation) string {
	return strings.TrimPrefix(secret.authorization, "Bearer ")
}

func createTransformationTemplate(ctx context.Context, aiUpstream *v1alpha1.AIUpstream) *envoytransformation.TransformationTemplate {
	// Setup initial transformation template. This may be modified by further
	transformationTemplate := &envoytransformation.TransformationTemplate{
		// We will add the auth token later
		Headers: map[string]*envoytransformation.InjaTemplate{},
	}

	var headerName, prefix, path string
	var bodyTransformation *envoytransformation.TransformationTemplate_MergeJsonKeys
	if aiUpstream.LLM != nil {
		headerName, prefix, path, bodyTransformation = getTransformation(ctx, aiUpstream.LLM)
	} else if aiUpstream.MultiPool != nil {
		// We already know that all the backends are the same type so we can just take the first one
		llmMultiPool := aiUpstream.MultiPool.Priorities[0].Pool[0]
		headerName, prefix, path, bodyTransformation = getTransformation(ctx, &llmMultiPool)
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

func getTransformation(ctx context.Context, llm *v1alpha1.LLMProviders) (string, string, string, *envoytransformation.TransformationTemplate_MergeJsonKeys) {
	headerName := "Authorization"
	var prefix, path string
	var bodyTransformation *envoytransformation.TransformationTemplate_MergeJsonKeys
	if llm.OpenAI != nil {
		prefix = "Bearer "
		path = "/v1/chat/completions"
		bodyTransformation = defaultBodyTransformation()
	} else if llm.Anthropic != nil {
		headerName = "x-api-key"
		path = "/v1/messages"
		bodyTransformation = defaultBodyTransformation()
	} else if llm.AzureOpenAI != nil {
		headerName = "api-key"
		path = `/openai/deployments/{{ host_metadata("model") }}/chat/completions?api-version={{ host_metadata("api_version" )}}`
	} else if llm.Gemini != nil {
		headerName = "key"
		path = getGeminiPath()
	} else if llm.VertexAI != nil {
		prefix = "Bearer "
		var modelPath string
		modelCall := llm.VertexAI.ModelPath
		if modelCall != nil {
			switch llm.VertexAI.Publisher {
			case v1alpha1.GOOGLE:
				modelPath = getVertexAIGeminiModelPath()
			default:
				// TODO(npolshak): add support for other publishers
				contextutils.LoggerFrom(ctx).Warnf("Unsupported Vertex AI publisher: %v. Defaulting to Google", llm.VertexAI.Publisher)
				modelPath = getVertexAIGeminiModelPath()
			}
		} else {
			// Use user provided model path
			modelPath = fmt.Sprintf(`models/{{host_metadata("model")}}:%s`, modelCall)
		}
		// https://${LOCATION}-aiplatform.googleapis.com/{VERSION}/projects/${PROJECT_ID}/locations/${LOCATION}/<model-path>
		path = fmt.Sprintf(`/{{host_metadata("api_version")}}/projects/{{host_metadata("project")}}/locations/{{host_metadata("location")}}/publishers/{{host_metadata("publisher")}}/%s`, modelPath)
	}
	return headerName, prefix, path, bodyTransformation
}

func getGeminiPath() string {
	return `/{{host_metadata("api_version")}}/models/{{host_metadata("model")}}:{% if host_metadata("route_type") == "CHAT_STREAMING" %}streamGenerateContent?key={{host_metadata("auth_token")}}&alt=sse{% else %}generateContent?key={{host_metadata("auth_token")}}{% endif %}`
}

func getVertexAIGeminiModelPath() string {
	return `models/{{host_metadata("model")}}:{% if host_metadata("route_type") == "CHAT_STREAMING" %}streamGenerateContent?alt=sse{% else %}generateContent{% endif %}`
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
