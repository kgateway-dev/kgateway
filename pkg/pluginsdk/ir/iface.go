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
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
)

type ListenerContext struct {
	Policy            PolicyIR
	PolicyAncestorRef gwv1.ParentReference
}

type RouteConfigContext struct {
	// No policy here, as you can't attach policies to route configs.
	// we will call every policy with this to set defaults.
}

type VirtualHostContext struct {
	Policy            PolicyIR
	TypedFilterConfig TypedFilterConfigMap
}

type TypedFilterConfigMap map[string]proto.Message

// AddTypedConfig SETS the config for a given key. // TODO: consider renaming to SetTypedConfig
func (r *TypedFilterConfigMap) AddTypedConfig(key string, v proto.Message) {
	if *r == nil {
		*r = make(TypedFilterConfigMap)
	}
	(*r)[key] = v
}

func (r *TypedFilterConfigMap) GetTypedConfig(key string) proto.Message {
	if r == nil || *r == nil {
		return nil
	}
	if v, ok := (*r)[key]; ok {
		return v
	}
	return nil
}

type RouteBackendContext struct {
	FilterChainName string
	Backend         *BackendObjectIR
	// TypedFilterConfig will be output on the Route or WeightedCluster level after all plugins have run
	TypedFilterConfig TypedFilterConfigMap
}

type RouteContext struct {
	FilterChainName string
	Policy          PolicyIR
	In              HttpRouteRuleMatchIR
	// TypedFilterConfig will be output on the Route level after all plugins have run
	TypedFilterConfig TypedFilterConfigMap
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
	// no policy applied - this is called for every backend in a route.
	// For this to work the backend needs to register itself as a policy. TODO: rethink this.
	// Note: TypedFilterConfig should be applied in the pCtx and is shared between ApplyForRoute, ApplyForBackend
	// and ApplyForRouteBacken (do not apply on the output route directly)
	ApplyForBackend(
		ctx context.Context,
		pCtx *RouteBackendContext,
		in HttpBackend,
		out *envoy_config_route_v3.Route,
	) error
	// Applies a policy attached to a specific Backend (via extensionRef on the BackendRef).
	// Note: TypedFilterConfig should be applied in the pCtx and is shared between ApplyForRoute, ApplyForBackend
	// and ApplyForRouteBackend
	ApplyForRouteBackend(
		ctx context.Context,
		policy PolicyIR,
		pCtx *RouteBackendContext,
	) error
	// called once per route rule if SupportsPolicyMerge returns false, otherwise this is called only
	// once on the value returned by MergePolicies.
	// Applies policy for an HTTPRoute that has a policy attached via a targetRef.
	// The output configures the envoy_config_route_v3.Route
	// Note: TypedFilterConfig should be applied in the pCtx and is shared between ApplyForRoute, ApplyForBackend
	// and ApplyForRouteBacken (do not apply on the output route directly)
	ApplyForRoute(
		ctx context.Context,
		pCtx *RouteContext,
		out *envoy_config_route_v3.Route) error

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

type GwTranslationCtx struct{}

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
	// Errors should be formatted for users, so do not include internal lib errors.
	// Instead use a well defined error such as ErrInvalidConfig
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

var ErrNotAttachable = fmt.Errorf("policy is not attachable to this object")

type PolicyRun interface {
	// Allocate state for single listener+rotue translation pass.
	NewGatewayTranslationPass(ctx context.Context, tctx GwTranslationCtx, reporter reports.Reporter) ProxyTranslationPass
	// Process cluster for a backend
	ProcessBackend(ctx context.Context, in BackendObjectIR, out *envoy_config_cluster_v3.Cluster) error
}
