package upstream

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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func applyAIBackend(ctx context.Context, aiUpstream *v1alpha1.AIUpstream, pCtx *ir.RouteBackendContext, in ir.HttpBackend, out *envoy_config_route_v3.Route) error {
	// Setup ext-proc route filter config, we will conditionally modify it based on certain route options.
	// A heavily used part of this config is the `GrpcInitialMetadata`.
	// This is used to add headers to the ext-proc request.
	// These headers are used to configure the AI server on a per-request basis.
	// This was the best available way to pass per-route configuration to the AI server.
	extProcRouteSettings := &envoy_ext_proc_v3.ExtProcPerRoute{
		Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
			Overrides: &envoy_ext_proc_v3.ExtProcOverrides{},
		},
	}

	var llmModel string
	byType := map[string]struct{}{}
	if aiUpstream.LLM != nil {
		llmModel = getUpstreamModel(aiUpstream.LLM, byType)
	} else if aiUpstream.MultiPool != nil {
		for _, priority := range aiUpstream.MultiPool.Priorities {
			for _, pool := range priority.Pool {
				llmModel = getUpstreamModel(&pool, byType)
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

	// Add things which require basic AI upstream.
	if out == nil {
		panic("!!")
	}
	if out.GetRoute() == nil {
		// initialize route action if not set
		out.Action = &envoy_config_route_v3.Route_Route{
			Route: &envoy_config_route_v3.RouteAction{},
		}
	}
	out.GetRoute().HostRewriteSpecifier = &envoy_config_route_v3.RouteAction_AutoHostRewrite{
		AutoHostRewrite: wrapperspb.Bool(true),
	}

	//We only want to add the transformation filter if we have a single AI backend
	//Otherwise we already have the transformation filter added by the weighted destination.
	transformation := createTransformationTemplate(ctx, aiUpstream)
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
	marshaled, err := utils.MessageToAny(&envoytransformation.RouteTransformations{
		Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{routeTransformation},
	})
	if err != nil {
		return err
	}
	// Sets the transformation for the Upstream. Will be updated in a route policy is attached.
	pCtx.AddTypedConfig(wellknown.TransformationFilterName, marshaled)

	extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
		&envoy_config_core_v3.HeaderValue{
			Key:   "x-llm-provider",
			Value: llmProvider,
		},
	)
	// If the Upstream specifies a model, add a header to the ext-proc request
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
