package ai

import (
	"context"
	"os"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"github.com/rotisserie/eris"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func ApplyAIBackend(ctx context.Context, aiBackend *v1alpha1.AIBackend, pCtx *ir.RouteBackendContext, out *envoy_config_route_v3.Route) error {
	// Setup ext-proc route filter config, we will conditionally modify it based on certain route options.
	// A heavily used part of this config is the `GrpcInitialMetadata`.
	// This is used to add headers to the ext-proc request.
	// These headers are used to configure the AI server on a per-request basis.
	// This was the best available way to pass per-route configuration to the AI server.
	extProcRouteSettingsProto := pCtx.GetTypedConfig(wellknown.AIExtProcFilterName)
	var extProcRouteSettings *envoy_ext_proc_v3.ExtProcPerRoute
	if extProcRouteSettingsProto == nil {
		extProcRouteSettings = &envoy_ext_proc_v3.ExtProcPerRoute{
			Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
				Overrides: &envoy_ext_proc_v3.ExtProcOverrides{},
			},
		}
	} else {
		extProcRouteSettings = extProcRouteSettingsProto.(*envoy_ext_proc_v3.ExtProcPerRoute)
	}

	var llmModel string
	byType := map[string]struct{}{}
	if aiBackend.LLM != nil {
		llmModel = getBackendModel(aiBackend.LLM, byType)
	} else if aiBackend.MultiPool != nil {
		for _, priority := range aiBackend.MultiPool.Priorities {
			for _, pool := range priority.Pool {
				llmModel = getBackendModel(&pool, byType)
			}
		}
	}

	if len(byType) != 1 {
		return eris.Errorf("multiple AI backend types found for single ai route %+v", byType)
	}

	// This is only len(1)
	var llmProvider string
	for k := range byType {
		llmProvider = k
	}

	// Add things which require basic AI backend.
	if out.GetRoute() == nil {
		// initialize route action if not set
		out.Action = &envoy_config_route_v3.Route_Route{
			Route: &envoy_config_route_v3.RouteAction{},
		}
	}
	// LLM providers (open ai, etc.) expect the auto host rewrite to be set
	out.GetRoute().HostRewriteSpecifier = &envoy_config_route_v3.RouteAction_AutoHostRewrite{
		AutoHostRewrite: wrapperspb.Bool(true),
	}

	//We only want to add the transformation filter if we have a single AI backend
	//Otherwise we already have the transformation filter added by the weighted destination.
	transformation := createTransformationTemplate(ctx, aiBackend)
	routeTransformation := &envoytransformation.RouteTransformations_RouteTransformation{
		Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
			RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
				RequestTransformation: &envoytransformation.Transformation{
					// Set this env var to true to log the request/response info for each transformation
					LogRequestResponseInfo: wrapperspb.Bool(os.Getenv("AI_PLUGIN_DEBUG_TRANSFORMATIONS") == "true"),
					TransformationType: &envoytransformation.Transformation_TransformationTemplate{
						TransformationTemplate: transformation,
					},
				},
			},
		},
	}
	// Sets the transformation for the backend. Can be updated in a route policy is attached.
	transformations := &envoytransformation.RouteTransformations{
		Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{routeTransformation},
	}
	pCtx.AddTypedConfig(wellknown.AIBackendTransformationFilterName, transformations)

	extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
		&envoy_config_core_v3.HeaderValue{
			Key:   "x-llm-provider",
			Value: llmProvider,
		},
	)
	// If the backend specifies a model, add a header to the ext-proc request
	// TODO: add support for multi pool setting different models for different pools
	if llmModel != "" {
		extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
			&envoy_config_core_v3.HeaderValue{
				Key:   "x-llm-model",
				Value: llmModel,
			})
	}

	// Add the x-request-id header to the ext-proc request.
	// This is an optimization to allow us to not have to wait for the headers request to
	// Initialize our logger/handler classes.
	extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
		&envoy_config_core_v3.HeaderValue{
			Key:   "x-request-id",
			Value: "%REQ(X-REQUEST-ID)%",
		},
	)

	pCtx.AddTypedConfig(wellknown.AIExtProcFilterName, extProcRouteSettings)
	return nil
}

func getBackendModel(llm *v1alpha1.LLMProvider, byType map[string]struct{}) string {
	llmModel := ""
	provider := llm.Provider
	if provider.OpenAI != nil {
		byType["openai"] = struct{}{}
		if provider.OpenAI.Model != nil {
			llmModel = *provider.OpenAI.Model
		}
	} else if provider.Anthropic != nil {
		byType["anthropic"] = struct{}{}
		if provider.Anthropic.Model != nil {
			llmModel = *provider.Anthropic.Model
		}
	} else if provider.AzureOpenAI != nil {
		byType["azure_openai"] = struct{}{}
		llmModel = provider.AzureOpenAI.DeploymentName
	} else if provider.Gemini != nil {
		byType["gemini"] = struct{}{}
		llmModel = provider.Gemini.Model
	} else if provider.VertexAI != nil {
		byType["vertex-ai"] = struct{}{}
		llmModel = provider.VertexAI.Model
	}
	return llmModel
}
