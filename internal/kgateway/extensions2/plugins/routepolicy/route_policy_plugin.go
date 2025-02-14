package routepolicy

import (
	"context"
	"time"

	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/runtime/schema"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

type routePolicy struct {
	ct   time.Time
	spec v1alpha1.RoutePolicySpec
}

func (d *routePolicy) CreationTime() time.Time {
	return d.ct
}

func (d *routePolicy) Equals(in any) bool {
	d2, ok := in.(*routePolicy)
	if !ok {
		return false
	}
	return d.spec == d2.spec
}

type routePolicyPluginGwPass struct {
}

func (p *routePolicyPluginGwPass) ApplyHCM(ctx context.Context, pCtx *ir.HcmContext, out *envoyhttp.HttpConnectionManager) error {
	// no op
	return nil
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionplug.Plugin {
	col := krtutil.SetupCollectionDynamic[v1alpha1.RoutePolicy](
		ctx,
		commoncol.Client,
		v1alpha1.SchemeGroupVersion.WithResource("routepolicies"),
		commoncol.KrtOpts.ToOptions("RoutePolicy")...,
	)
	gk := v1alpha1.RoutePolicyGVK.GroupKind()
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.RoutePolicy) *ir.PolicyWrapper {
		var pol = &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: policyCR.Namespace,
				Name:      policyCR.Name,
			},
			Policy:     policyCR,
			PolicyIR:   &routePolicy{ct: policyCR.CreationTimestamp.Time, spec: policyCR.Spec},
			TargetRefs: convert(policyCR.Spec.TargetRef),
		}
		return pol
	})

	return extensionplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			v1alpha1.RoutePolicyGVK.GroupKind(): {
				//AttachmentPoints: []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				NewGatewayTranslationPass: NewGatewayTranslationPass,
				Policies:                  policyCol,
			},
		},
	}
}

func convert(targetRef v1alpha1.LocalPolicyTargetReference) []ir.PolicyTargetRef {
	return []ir.PolicyTargetRef{{
		Kind:  string(targetRef.Kind),
		Name:  string(targetRef.Name),
		Group: string(targetRef.Group),
	}}
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &routePolicyPluginGwPass{}
}
func (p *routePolicy) Name() string {
	return "routepolicies"
}

// called 1 time for each listener
func (p *routePolicyPluginGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *routePolicyPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *routePolicyPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	policy, ok := pCtx.Policy.(*routePolicy)
	if !ok {
		return nil
	}

	if policy.spec.Timeout > 0 && outputRoute.GetRoute() != nil {
		outputRoute.GetRoute().Timeout = durationpb.New(time.Second * time.Duration(policy.spec.Timeout))
	}

	return nil
}

func (p *routePolicyPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *routePolicyPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	return nil, nil
}

func (p *routePolicyPluginGwPass) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *routePolicyPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *routePolicyPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}
