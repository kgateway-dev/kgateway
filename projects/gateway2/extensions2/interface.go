package extensions

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/model"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	anypb "google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ListenerContext struct{}
type VirtualHostContext struct {
	Policy metav1.Object
}
type RouteBackendContext struct {
	FilterChainName string
	Upstream        model.Upstream
	// todo: make this not public
	TypedFiledConfig *map[string]*anypb.Any
}

func (r *RouteBackendContext) AddTypedConfig(key string, v *anypb.Any) {
	if *r.TypedFiledConfig == nil {
		*r.TypedFiledConfig = make(map[string]*anypb.Any)
	}
	(*r.TypedFiledConfig)[key] = v
}

type RouteContext struct {
	Policy   metav1.Object
	Reporter reports.ParentRefReporter
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
	ApplyForRouteBackend(
		ctx context.Context,
		pCtx *RouteBackendContext,
		policy metav1.Object,
	) error
	// called 1 time per listener
	// if a plugin emits new filters, they must be with a plugin unique name.
	// any filter returned from route config must be disabled, so it doesnt impact other routes.
	HttpFilters(ctx context.Context, fc model.FilterChainCommon) ([]plugins.StagedHttpFilter, error)
	UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error)

	NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error)
	// called 1 time (per envoy proxy). replaces GeneratedResources
	ResourcesToAdd(ctx context.Context) Resources
}

type Resources struct {
	Clusters []envoy_config_cluster_v3.Cluster
}

type GwTranslationCtx struct{}

type PolicyImpl struct {
	AttachmentPoints          []model.AttachmentPoints
	NewGatewayTranslationPass func(ctx context.Context, tctx GwTranslationCtx) ProxyTranslationPass
	Policies                  krt.Collection[model.Policy]
	PoliciesFetch             func(n, ns string) model.Policy
	ProcessUpstream           func(ctx context.Context, policy metav1.Object, in model.Upstream, out *envoy_config_cluster_v3.Cluster)
}
type UpstreamImpl struct {
	ProcessUpstream func(ctx context.Context, in model.Upstream, out *envoy_config_cluster_v3.Cluster)
	Upstreams       krt.Collection[model.Upstream]
	Endpoints       krt.Collection[krtcollections.EndpointsForUpstream]
}
type Plugin struct {
	ContributesPolicies  map[schema.GroupKind]PolicyImpl
	ContributesUpstreams map[schema.GroupKind]UpstreamImpl
	ContributesGwClasses map[string]interface {
		// TranslateProxy This function is called by the reconciler when a K8s Gateway resource is created or updated.
		// It returns an instance of the k8sgateway Proxy resource, that should configure a target k8sgateway Proxy workload.
		// A null return value indicates the K8s Gateway resource failed to translate into a k8sgateway Proxy. The error will be reported on the provided reporter.
		TranslateProxy(
			ctx context.Context,
			gateway *gwv1.Gateway,
			writeNamespace string,
			reporter reports.Reporter,
		) *model.GatewayIR
	}
}

type K8sGatewayExtensions2 struct {
	Plugins []Plugin
}
