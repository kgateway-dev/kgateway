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

type RouteConfigContext struct {
	// No policy here, as you can't attach policies to route configs.
	// we will call every policy with this to set defaults.
}

type VirtualHostContext struct {
	Policy PolicyIR
}
type RouteBackendContext struct {
	FilterChainName string
	Backend         *BackendObjectIR
	// todo: make this not public
	TypedFilterConfig *map[string]proto.Message
}

func (r *RouteBackendContext) AddTypedConfig(key string, v proto.Message) {
	if *r.TypedFilterConfig == nil {
		*r.TypedFilterConfig = make(map[string]proto.Message)
	}
	(*r.TypedFilterConfig)[key] = v
}

func (r *RouteBackendContext) GetTypedConfig(key string) proto.Message {
	if *r.TypedFilterConfig == nil {
		return nil
	}
	return (*r.TypedFilterConfig)[key]
}

type RouteContext struct {
	FilterChainName string
	Policy          PolicyIR
	In              HttpRouteRuleMatchIR
}

type HcmContext struct {
	Policy PolicyIR
}

// ProxyTranslationPass represents a single translation pass for a gateway. It can hold state
// for the duration of the translation.
// Each of the functions here will be called in the order they appear in the interface.
type ProxyTranslationPass interface {
	//	Name() string
	// called 1 time for each listener
	ApplyListenerPlugin(
		ctx context.Context,
		pCtx *ListenerContext,
		out *envoy_config_listener_v3.Listener,
	)
	// called 1 time per filter chain after listeners and allows tweaking HCM settings.
	ApplyHCM(ctx context.Context,
		pCtx *HcmContext,
		out *envoy_hcm.HttpConnectionManager) error

	// called 1 time for all the routes in a filter chain. Use this to set default PerFilterConfig
	// No policy is provided here.
	ApplyRouteConfigPlugin(
		ctx context.Context,
		pCtx *RouteConfigContext,
		out *envoy_config_route_v3.RouteConfiguration,
	)
	ApplyVhostPlugin(
		ctx context.Context,
		pCtx *VirtualHostContext,
		out *envoy_config_route_v3.VirtualHost,
	)
	// called 0 or more times (one for each route)
	// Applies policy for an HTTPRoute that has a policy attached via a targetRef.
	// The output configures the envoy_config_route_v3.Route
	ApplyForRoute(
		ctx context.Context,
		pCtx *RouteContext,
		out *envoy_config_route_v3.Route) error

	// no policy applied - this is called for every backend in a route.
	// For this to work the backend needs to register itself as a policy. TODO: rethink this.
	ApplyForBackend(
		ctx context.Context,
		pCtx *RouteBackendContext,
		in HttpBackend,
		out *envoy_config_route_v3.Route,
	) error

	// Applies a policy attached to a specific Backend (via extensionRef on the BackendRef).
	ApplyForRouteBackend(
		ctx context.Context,
		policy PolicyIR,
		pCtx *RouteBackendContext,
	) error

	// called 1 time per filter-chain.
	// If a plugin emits new filters, they must be with a plugin unique name.
	// filters added to impact specific routes should be disabled on the listener level, so they don't impact other routes.
	HttpFilters(ctx context.Context, fc FilterChainCommon) ([]plugins.StagedHttpFilter, error)

	NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error)
	// called 1 time (per envoy proxy). replaces GeneratedResources and allows adding clusters to the envoy.
	ResourcesToAdd(ctx context.Context) Resources
}

type UnimplementedProxyTranslationPass struct{}

var _ ProxyTranslationPass = UnimplementedProxyTranslationPass{}

func (s UnimplementedProxyTranslationPass) ApplyListenerPlugin(ctx context.Context, pCtx *ListenerContext, out *envoy_config_listener_v3.Listener) {
}
func (s UnimplementedProxyTranslationPass) ApplyHCM(ctx context.Context, pCtx *HcmContext, out *envoy_hcm.HttpConnectionManager) error {
	return nil
}
func (s UnimplementedProxyTranslationPass) ApplyForBackend(ctx context.Context, pCtx *RouteBackendContext, in HttpBackend, out *envoy_config_route_v3.Route) error {
	return nil
}
func (s UnimplementedProxyTranslationPass) ApplyRouteConfigPlugin(ctx context.Context, pCtx *RouteConfigContext, out *envoy_config_route_v3.RouteConfiguration) {
}

func (s UnimplementedProxyTranslationPass) ApplyVhostPlugin(ctx context.Context, pCtx *VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}
func (s UnimplementedProxyTranslationPass) ApplyForRoute(ctx context.Context, pCtx *RouteContext, out *envoy_config_route_v3.Route) error {
	return nil
}
func (s UnimplementedProxyTranslationPass) ApplyForRouteBackend(ctx context.Context, policy PolicyIR, pCtx *RouteBackendContext) error {
	return nil
}
func (s UnimplementedProxyTranslationPass) HttpFilters(ctx context.Context, fc FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	return nil, nil
}
func (s UnimplementedProxyTranslationPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}
func (s UnimplementedProxyTranslationPass) ResourcesToAdd(ctx context.Context) Resources {
	return Resources{}
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
	// A reference to the original policy object
	ObjectSource `json:",inline"`
	// The policy object itself. TODO: we can probably remove this
	Policy metav1.Object

	// Errors processing it for status.
	// note: these errors are based on policy itself, regardless of whether it's attached to a resource.
	// TODO: change for conditions
	Errors []error

	// The IR of the policy objects. ideally with structural errors removed.
	// Opaque to us other than metadata.
	PolicyIR PolicyIR

	// Where to attach the policy. This usually comes from the policy CRD.
	TargetRefs []PolicyRef
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

	return versionEquals(c.Policy, in.Policy) && c.PolicyIR.Equals(in.PolicyIR)
}

var (
	ErrNotAttachable = fmt.Errorf("policy is not attachable to this object")
)

type PolicyRun interface {
	// Allocate state for single listener+rotue translation pass.
	NewGatewayTranslationPass(ctx context.Context, tctx GwTranslationCtx) ProxyTranslationPass
	// Process cluster for a backend
	ProcessBackend(ctx context.Context, in BackendObjectIR, out *envoy_config_cluster_v3.Cluster) error
}
