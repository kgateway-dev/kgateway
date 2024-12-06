package extensions

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/model"
	"github.com/solo-io/gloo/projects/gateway2/translator"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ListenerContext struct{}
type VirtualHostContext struct {
	Policy metav1.Object
}
type RouteContext struct {
	Policy metav1.Object
}

type ProxyTranslationPass interface {
	Name() string
	// called 1 time for each listener
	ApplyListenerPlugin(
		ctx context.Context,
		pCtx *ListenerContext,
		out *envoy_config_listener_v3.Listener,
	)

	ApplyVhostPlugin(
		ctx context.Context,
		pCtx *VirtualHostContext,
		out *envoy_config_route_v3.VirtualHost,
	)
	// called 0 or more times
	ApplyForRoute(
		ctx context.Context,
		pCtx *RouteContext,
		out *envoy_config_route_v3.Route) error
	// called 1 time per listener
	// if a plugin emits new filters, they must be with a plugin unique name.
	// any filter returned from route config must be disabled, so it doesnt impact other routes.
	HttpFilters(ctx context.Context) ([]plugins.StagedHttpFilter, error)
	UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error)

	NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error)
	// called 1 time (per envoy proxy). replaces GeneratedResources
	ResourcesToAdd(ctx context.Context) Resources
}

type Resources struct {
	Clusters []envoy_config_cluster_v3.Cluster
}

type GwTranslationCtx struct{}

type Plugin struct {
	ContributesPolicies map[schema.GroupKind]struct {
		AttachmentPoints          []model.AttachmentPoints
		NewGatewayTranslationPass func(ctx context.Context, tctx GwTranslationCtx) ProxyTranslationPass
		Policies                  krt.Collection[model.Policy]
	}

	ContributesUpstreams map[schema.GroupKind]struct {
		ProcessUpstream func(ctx context.Context, in model.Upstream, out *envoy_config_cluster_v3.Cluster)
		Upstreams       krt.Collection[model.Upstream]
		Endpoints       []krt.Collection[krtcollections.EndpointsForUpstream]
	}
	ContributesGwClasses map[string]translator.K8sGwTranslator
}

type K8sGatewayExtensions2 struct {
	Plugins []Plugin
}
