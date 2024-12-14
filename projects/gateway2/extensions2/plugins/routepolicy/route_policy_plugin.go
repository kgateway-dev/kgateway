package routepolicy

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/runtime/schema"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/common"
	extensionplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"istio.io/istio/pkg/kube/krt"
)

type routeOptsPlugin struct {
	ct   time.Time
	spec v1alpha1.RoutePolicySpec
}

func (d *routeOptsPlugin) CreationTime() time.Time {
	return d.ct
}

func (d *routeOptsPlugin) Equals(in any) bool {
	d2, ok := in.(*routeOptsPlugin)
	if !ok {
		return false
	}
	return d.spec == d2.spec
}

type routeOptsPluginGwPass struct {
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionplug.Plugin {

	col := krtutil.SetupCollectionDynamic[v1alpha1.RoutePolicy](
		ctx,
		commoncol.Client,
		v1alpha1.SchemeGroupVersion.WithResource("routepolicies"),
		commoncol.KrtOpts.ToOptions("RoutePolicy")...,
	)
	gk := v1alpha1.RoutePolicyGVK.GroupKind()
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.RoutePolicy) *ir.PolicyWrapper {
		var pol = &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Policy:     i,
			PolicyIR:   &routeOptsPlugin{ct: i.CreationTimestamp.Time, spec: i.Spec},
			TargetRefs: convert(i.Spec.TargetRef),
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
	return &routeOptsPluginGwPass{}
}
func (p *routeOptsPlugin) Name() string {
	return "routepolicies"
}

// called 1 time for each listener
func (p *routeOptsPluginGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *routeOptsPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *routeOptsPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	policy, ok := pCtx.Policy.(*routeOptsPlugin)
	if !ok {
		return nil
	}

	if policy.spec.Timeout > 0 && outputRoute.GetRoute() != nil {
		outputRoute.GetRoute().Timeout = durationpb.New(time.Second * time.Duration(policy.spec.Timeout))
	}

	return nil
}

func (p *routeOptsPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *routeOptsPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	return nil, nil
}

func (p *routeOptsPluginGwPass) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *routeOptsPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *routeOptsPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}
