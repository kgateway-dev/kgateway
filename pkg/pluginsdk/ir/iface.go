package ir

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/agentgateway/agentgateway/go/api"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var logger = logging.New("pluginsdk/ir")

type GatewayContext struct {
	GatewayClassName string
}

type ListenerContext struct {
	Policy            PolicyIR
	PolicyAncestorRef gwv1.ParentReference
}

type RouteConfigContext struct {
	Policy            PolicyIR
	FilterChainName   string
	TypedFilterConfig TypedFilterConfigMap
	GatewayContext    GatewayContext
}

type VirtualHostContext struct {
	Policy            PolicyIR
	FilterChainName   string
	TypedFilterConfig TypedFilterConfigMap
	GatewayContext    GatewayContext
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

func (r *TypedFilterConfigMap) ToAnyMap() map[string]*anypb.Any {
	typedPerFilterConfigAny := map[string]*anypb.Any{}
	for k, v := range *r {
		if anyMsg, ok := v.(*anypb.Any); ok {
			typedPerFilterConfigAny[k] = anyMsg
			continue
		}
		config, err := utils.MessageToAny(v)
		if err != nil {
			logger.Error("unexpected marshalling error", "error", err)
			continue
		}
		typedPerFilterConfigAny[k] = config
	}
	return typedPerFilterConfigAny
}

type RouteBackendContext struct {
	GatewayContext  GatewayContext
	FilterChainName string
	Backend         *BackendObjectIR
	// TypedFilterConfig will be output on the Route or WeightedCluster level after all plugins have run
	TypedFilterConfig       TypedFilterConfigMap
	RequestHeadersToAdd     []*envoycorev3.HeaderValueOption
	RequestHeadersToRemove  []string
	ResponseHeadersToAdd    []*envoycorev3.HeaderValueOption
	ResponseHeadersToRemove []string
}

type RouteContext struct {
	FilterChainName string
	Policy          PolicyIR
	GatewayContext  GatewayContext
	In              HttpRouteRuleMatchIR
	// TypedFilterConfig will be output on the Route level after all plugins have run
	TypedFilterConfig TypedFilterConfigMap

	InheritedPolicyPriority apiannotations.InheritedPolicyPriorityValue
}

type HcmContext struct {
	Policy PolicyIR
}

// ProxyTranslationPass represents a single translation pass for a gateway using envoy. It can hold state
// for the duration of the translation.
// Each of the functions here will be called in the order they appear in the interface.
type ProxyTranslationPass interface {
	//	Name() string
	// called 1 time for each listener
	ApplyListenerPlugin(
		ctx context.Context,
		pCtx *ListenerContext,
		out *envoylistenerv3.Listener,
	)

	// called 1 time for all the routes in a filter chain. Use this to set default PerFilterConfig
	// No policy is provided here.
	ApplyRouteConfigPlugin(
		ctx context.Context,
		pCtx *RouteConfigContext,
		out *envoyroutev3.RouteConfiguration,
	)

	// no policy applied - this is called for every backend in a route.
	// For this to work the backend needs to register itself as a policy. TODO: rethink this.
	// Note: TypedFilterConfig should be applied in the pCtx and is shared between ApplyForRoute, ApplyForBackend
	// and ApplyForRouteBacken (do not apply on the output route directly)
	ApplyForBackend(
		ctx context.Context,
		pCtx *RouteBackendContext,
		in HttpBackend,
		out *envoyroutev3.Route,
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
	// The output configures the envoyroutev3.Route
	// Note: TypedFilterConfig should be applied in the pCtx and is shared between ApplyForRoute, ApplyForBackend
	// and ApplyForRouteBacken (do not apply on the output route directly)
	ApplyForRoute(
		ctx context.Context,
		pCtx *RouteContext,
		out *envoyroutev3.Route,
	) error

	ApplyVhostPlugin(
		ctx context.Context,
		pCtx *VirtualHostContext,
		out *envoyroutev3.VirtualHost,
	)

	NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error)

	// called 1 time per filter-chain.
	// If a plugin emits new filters, they must be with a plugin unique name.
	// filters added to impact specific routes should be disabled on the listener level, so they don't impact other routes.
	HttpFilters(ctx context.Context, fc FilterChainCommon) ([]plugins.StagedHttpFilter, error)

	// called 1 time per filter chain after listeners and allows tweaking HCM settings.
	ApplyHCM(ctx context.Context,
		pCtx *HcmContext,
		out *envoy_hcm.HttpConnectionManager) error

	// called 1 time (per envoy proxy). replaces GeneratedResources and allows adding clusters to the envoy.
	ResourcesToAdd(ctx context.Context) Resources
}

type AgentGatewayRouteContext struct {
	Rule *gwv1.HTTPRouteRule
}

type AgentGatewayTranslationBackendContext struct {
	Backend        *BackendObjectIR
	GatewayContext GatewayContext
}

type AgentGatewayTranslationPass interface {
	// ApplyForRoute processes route-level configuration
	ApplyForRoute(pCtx *AgentGatewayRouteContext, out *api.Route) error

	// ApplyForBackend processes backend-level configuration for each backend referenced in routes
	ApplyForBackend(pCtx *AgentGatewayTranslationBackendContext, out *api.Backend) error

	// ApplyForRouteBackend processes route-specific backend configuration
	ApplyForRouteBackend(policy PolicyIR, pCtx *AgentGatewayTranslationBackendContext) error
}

type UnimplementedProxyTranslationPass struct{}

var _ ProxyTranslationPass = UnimplementedProxyTranslationPass{}

type UnimplementedAgentGatewayTranslationPass struct{}

var _ AgentGatewayTranslationPass = UnimplementedAgentGatewayTranslationPass{}

func (s UnimplementedAgentGatewayTranslationPass) ApplyForRoute(pCtx *AgentGatewayRouteContext, out *api.Route) error {
	return nil
}

func (s UnimplementedAgentGatewayTranslationPass) ApplyForBackend(pCtx *AgentGatewayTranslationBackendContext, out *api.Backend) error {
	return nil
}

func (s UnimplementedAgentGatewayTranslationPass) ApplyForRouteBackend(policy PolicyIR, pCtx *AgentGatewayTranslationBackendContext) error {
	return nil
}

func (s UnimplementedProxyTranslationPass) ApplyListenerPlugin(ctx context.Context, pCtx *ListenerContext, out *envoylistenerv3.Listener) {
}

func (s UnimplementedProxyTranslationPass) ApplyHCM(ctx context.Context, pCtx *HcmContext, out *envoy_hcm.HttpConnectionManager) error {
	return nil
}

func (s UnimplementedProxyTranslationPass) ApplyForBackend(ctx context.Context, pCtx *RouteBackendContext, in HttpBackend, out *envoyroutev3.Route) error {
	return nil
}

func (s UnimplementedProxyTranslationPass) ApplyRouteConfigPlugin(ctx context.Context, pCtx *RouteConfigContext, out *envoyroutev3.RouteConfiguration) {
}

func (s UnimplementedProxyTranslationPass) ApplyVhostPlugin(ctx context.Context, pCtx *VirtualHostContext, out *envoyroutev3.VirtualHost) {
}

func (s UnimplementedProxyTranslationPass) ApplyForRoute(ctx context.Context, pCtx *RouteContext, out *envoyroutev3.Route) error {
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
	Clusters []*envoyclusterv3.Cluster
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

	if !slices.EqualFunc(c.Errors, in.Errors, func(e1, e2 error) bool {
		if e1 == nil && e2 != nil {
			return false
		}
		if e1 != nil && e2 == nil {
			return false
		}
		if (e1 != nil && e2 != nil) && e1.Error() != e2.Error() {
			return false
		}

		return true
	}) {
		return false
	}

	return versionEquals(c.Policy, in.Policy) && c.PolicyIR.Equals(in.PolicyIR)
}

var ErrNotAttachable = fmt.Errorf("policy is not attachable to this object")

type PolicyRun interface {
	// Allocate state for single listener+rotue translation pass.
	NewGatewayTranslationPass(ctx context.Context, tctx GwTranslationCtx, reporter reports.Reporter) ProxyTranslationPass
	// Process cluster for a backend
	ProcessBackend(ctx context.Context, in BackendObjectIR, out *envoyclusterv3.Cluster) error
}
