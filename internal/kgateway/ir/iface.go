package ir

import (
	"context"
	"fmt"
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
)

type ListenerContext struct {
	Policy PolicyIR
}
type VirtualHostContext struct {
	Policy PolicyIR
}
type RouteBackendContext struct {
	FilterChainName string
	Upstream        *Upstream
	// todo: make this not public
	TypedFilterConfig *map[string]proto.Message
}

func (r *RouteBackendContext) AddTypedConfig(key string, v proto.Message) {
	if *r.TypedFilterConfig == nil {
		*r.TypedFilterConfig = make(map[string]proto.Message)
	}
	(*r.TypedFilterConfig)[key] = v
}

func (r *RouteBackendContext) GetConfig(key string) proto.Message {
	if *r.TypedFilterConfig == nil {
		return nil
	}
	return (*r.TypedFilterConfig)[key]
}

type RouteContext struct {
	Policy PolicyIR
	In     HttpRouteRuleMatchIR
}

type HcmContext struct {
	Policy PolicyIR
}

type ProxyTranslationPass interface {
	//	Name() string
	// called 1 time for each listener
	ApplyListenerPlugin(
		ctx context.Context,
		pCtx *ListenerContext,
		out *envoy_config_listener_v3.Listener,
	)
	// called 1 time per filter chain after listeners
	ApplyHCM(ctx context.Context,
		pCtx *HcmContext,
		out *envoy_hcm.HttpConnectionManager) error

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
	// runs for policy applied
	ApplyForRouteBackend(
		ctx context.Context,
		policy PolicyIR,
		pCtx *RouteBackendContext,
	) error
	// no policy applied
	ApplyForBackend(
		ctx context.Context,
		pCtx *RouteBackendContext,
		in HttpBackend,
		out *envoy_config_route_v3.Route,
	) error

	// called 1 time per listener
	// if a plugin emits new filters, they must be with a plugin unique name.
	// any filter returned from route config must be disabled, so it doesnt impact other routes.
	HttpFilters(ctx context.Context, fc FilterChainCommon) ([]plugins.StagedHttpFilter, error)
	UpstreamHttpFilters(ctx context.Context, fc FilterChainCommon) ([]plugins.StagedUpstreamHttpFilter, error)

	NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error)
	// called 1 time (per envoy proxy). replaces GeneratedResources
	ResourcesToAdd(ctx context.Context) Resources
}

type Resources struct {
	Clusters []*envoy_config_cluster_v3.Cluster
}

type GwTranslationCtx struct {
}

type PolicyIR interface {
	// in case multiple policies attached to the same resource, we sort by policy creation time.
	CreationTime() time.Time
	Equals(in any) bool
}

type PolicyWrapper struct {
	ObjectSource `json:",inline"`
	Policy       metav1.Object

	// Errors processing it for status.
	// note: these errors are based on policy itself, regardless of whether it's attached to a resource.
	// TODO: change for conditions
	Errors []error

	// original object. ideally with structural errors removed.
	// Opaque to us other than metadata.
	PolicyIR PolicyIR

	TargetRefs []PolicyTargetRef
}

func (c PolicyWrapper) ResourceName() string {
	return c.ObjectSource.ResourceName()
}

func versionEquals(a, b metav1.Object) bool {
	var versionEquals bool
	if a.GetGeneration() != 0 && b.GetGeneration() != 0 {
		versionEquals = a.GetGeneration() == b.GetGeneration()
	} else {
		versionEquals = a.GetResourceVersion() == b.GetResourceVersion()
	}
	return versionEquals && a.GetUID() == b.GetUID()
}

func (c PolicyWrapper) Equals(in PolicyWrapper) bool {
	if c.ObjectSource != in.ObjectSource {
		return false
	}

	return versionEquals(c.Policy, in.Policy)
}

var (
	ErrNotAttachable = fmt.Errorf("policy is not attachable to this object")
)

type PolicyRun interface {
	NewGatewayTranslationPass(ctx context.Context, tctx GwTranslationCtx) ProxyTranslationPass
	ProcessUpstream(ctx context.Context, in Upstream, out *envoy_config_cluster_v3.Cluster) error
}
