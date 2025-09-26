package ai

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/trafficpolicy"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// TODO: envoy-based AI gateway is deprecated in v2.1 and will be removed in v2.2. The files in this folder (and any associated tests) can be removed in v2.2.

// IR is the internal representation of an AI backend.
type IR struct {
	AISecret       *ir.Secret
	AIMultiSecret  map[string]*ir.Secret
	Transformation *envoytransformation.RouteTransformations
	Extproc        *envoy_ext_proc_v3.ExtProcPerRoute
}

func (i *IR) Equals(otherAIIr *IR) bool {
	if i != nil {
		if otherAIIr == nil {
			// one of i or otherAIIr is nil, not equal
			return false
		}

		if !maps.EqualFunc(data(i.AISecret), data(otherAIIr.AISecret), func(a, b []byte) bool {
			return bytes.Equal(a, b)
		}) {
			return false
		}
		if !maps.EqualFunc(i.AIMultiSecret, otherAIIr.AIMultiSecret, func(a, b *ir.Secret) bool {
			return maps.EqualFunc(data(a), data(b), func(a, b []byte) bool {
				return bytes.Equal(a, b)
			})
		}) {
			return false
		}
		if !proto.Equal(i.Extproc, otherAIIr.Extproc) {
			return false
		}
		if !proto.Equal(i.Transformation, otherAIIr.Transformation) {
			return false
		}
	}
	return true
}

func data(s *ir.Secret) map[string][]byte {
	if s == nil {
		return nil
	}
	return s.Data
}

func ApplyAIBackend(ir *IR, pCtx *ir.RouteBackendContext, out *envoyroutev3.Route) error {
	pCtx.TypedFilterConfig.AddTypedConfig(wellknown.AIBackendTransformationFilterName, ir.Transformation)

	copyBackendExtproc := proto.Clone(ir.Extproc).(*envoy_ext_proc_v3.ExtProcPerRoute)
	trafficpolicyExtprocSettingsProto := pCtx.TypedFilterConfig.GetTypedConfig(wellknown.AIExtProcFilterName)
	if trafficpolicyExtprocSettingsProto != nil {
		// merge the Backend extproc config with any config added by the TrafficPolicy
		routeExtprocSettings := trafficpolicyExtprocSettingsProto.(*envoy_ext_proc_v3.ExtProcPerRoute)
		copyBackendExtproc.GetOverrides().GrpcInitialMetadata = append(copyBackendExtproc.GetOverrides().GetGrpcInitialMetadata(), routeExtprocSettings.GetOverrides().GetGrpcInitialMetadata()...)
	}
	pCtx.TypedFilterConfig.AddTypedConfig(wellknown.AIExtProcFilterName, copyBackendExtproc)

	// Add things which require basic AI backend.
	if out.GetRoute() == nil {
		// initialize route action if not set
		out.Action = &envoyroutev3.Route_Route{
			Route: &envoyroutev3.RouteAction{},
		}
	}
	// LLM providers (open ai, etc.) expect the auto host rewrite to be set
	out.GetRoute().HostRewriteSpecifier = &envoyroutev3.RouteAction_AutoHostRewrite{
		AutoHostRewrite: wrapperspb.Bool(true),
	}

	return nil
}

func PreprocessAIBackend(ctx context.Context, aiBackend *v1alpha1.AIBackend, ir *IR) error {
	// Setup ext-proc route filter config, we will conditionally modify it based on certain route options.
	// A heavily used part of this config is the `GrpcInitialMetadata`.
	// This is used to add headers to the ext-proc request.
	// These headers are used to configure the AI server on a per-request basis.
	// This was the best available way to pass per-route configuration to the AI server.
	extProcRouteSettings := ir.Extproc
	if extProcRouteSettings == nil {
		extProcRouteSettings = &envoy_ext_proc_v3.ExtProcPerRoute{
			Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
				Overrides: &envoy_ext_proc_v3.ExtProcOverrides{},
			},
		}
	}

	var llmModel string
	byType := map[string]struct{}{}
	if aiBackend.LLM != nil {
		llmModel = getBackendModel(aiBackend.LLM, byType)
	} else {
		for _, group := range aiBackend.PriorityGroups {
			for _, provider := range group.Providers {
				llmModel = getBackendModel(&provider.LLMProvider, byType)
			}
		}
	}

	if len(byType) != 1 {
		return fmt.Errorf("multiple AI backend types found for single ai route %+v", byType)
	}

	// This is only len(1)
	var llmProvider string
	for k := range byType {
		llmProvider = k
	}

	// We only want to add the transformation filter if we have a single AI backend
	// Otherwise we already have the transformation filter added by the weighted destination.
	transformation := createTransformationTemplate(aiBackend)
	routeTransformation := &envoytransformation.RouteTransformations_RouteTransformation{
		Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
			RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
				RequestTransformation: &envoytransformation.Transformation{
					// Set this env var to true to log the request/response info for each transformation
					LogRequestResponseInfo: wrapperspb.Bool(os.Getenv(trafficpolicy.AiDebugTransformations) == "true"),
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
	// Store transformations in IR
	ir.Transformation = transformations

	extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
		&envoycorev3.HeaderValue{
			Key:   "x-llm-provider",
			Value: llmProvider,
		},
	)
	// If the backend specifies a model, add a header to the ext-proc request
	// TODO: add support for multi pool setting different models for different pools
	if llmModel != "" {
		extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
			&envoycorev3.HeaderValue{
				Key:   "x-llm-model",
				Value: llmModel,
			})
	}

	// Add the x-request-id header to the ext-proc request.
	// This is an optimization to allow us to not have to wait for the headers request to
	// Initialize our logger/handler classes.
	extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
		&envoycorev3.HeaderValue{
			Key:   "x-request-id",
			Value: "%REQ(X-REQUEST-ID)%",
		},
	)

	// Store extproc settings in IR
	ir.Extproc = extProcRouteSettings

	return nil
}

func getBackendModel(provider *v1alpha1.LLMProvider, byType map[string]struct{}) string {
	llmModel := ""
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
	} else if provider.Bedrock != nil {
		// currently only supported in agentgateway
		byType["bedrock"] = struct{}{}
		llmModel = provider.Bedrock.Model
	}
	return llmModel
}
