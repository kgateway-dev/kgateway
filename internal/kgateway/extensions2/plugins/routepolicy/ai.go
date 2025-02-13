package routepolicy

import (
	"context"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

const (
	aiMetadataNamespace        = "envoy.filters.ai.solo.io"
	transformationFilterName   = "ai.transformation.solo.io"
	transformationEEFilterName = "ai.transformation_ee.solo.io"
	extProcFilterName          = "ai.extproc.solo.io"
	modSecurityFilterName      = "ai.modsecurity.solo.io"
	setMetadataFilterName      = "envoy.filters.http.set_filter_state"
	extProcUDSClusterName      = "ai_ext_proc_uds_cluster"
	extProcUDSSocketPath       = "@gloo-ai-sock"
)

func processAIRoutePolicy(ctx context.Context, aiConfig *v1alpha1.AIRoutePolicy, outputRoute *envoy_config_route_v3.Route, pCtx *ir.RouteContext) error {
	if outputRoute.GetTypedPerFilterConfig() == nil {
		outputRoute.TypedPerFilterConfig = make(map[string]*anypb.Any)
	}



	upstreams := getAiUpstreams(pCtx.In.Backends)
	if len(upstreams) != 0 {

	}

	return nil
}

func getAiUpstreams(backends []ir.HttpBackend) []*ir.Upstream {
	var upstreams []*ir.Upstream
	for _, backend := range backends {
		if backend.Backend.Upstream.ObjIr
	}
	return upstreams
}
