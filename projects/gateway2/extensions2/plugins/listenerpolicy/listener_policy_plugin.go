package listenerpolicy

import (
	"context"
	"time"

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
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
)

type listenerOptsPlugin struct {
	ct   time.Time
	spec v1alpha1.ListenerPolicySpec
}

func (d *listenerOptsPlugin) CreationTime() time.Time {
	return d.ct
}

func (d *listenerOptsPlugin) Equals(in any) bool {
	d2, ok := in.(*listenerOptsPlugin)
	if !ok {
		return false
	}
	return d.spec == d2.spec
}

type listenerOptsPluginGwPass struct {
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionplug.Plugin {

	col := krtutil.SetupCollectionDynamic[v1alpha1.ListenerPolicy](
		ctx,
		commoncol.Client,
		v1alpha1.SchemeGroupVersion.WithResource("listenerpolicies"),
		commoncol.KrtOpts.ToOptions("ListenerPolicy")...,
	)
	gk := v1alpha1.ListenerPolicyGVK.GroupKind()
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.ListenerPolicy) *ir.PolicyWrapper {
		var pol = &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Policy:     i,
			PolicyIR:   &listenerOptsPlugin{ct: i.CreationTimestamp.Time, spec: i.Spec},
			TargetRefs: convert(i.Spec.TargetRef),
		}
		return pol
	})

	return extensionplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			v1alpha1.ListenerPolicyGVK.GroupKind(): {
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
	return &listenerOptsPluginGwPass{}
}
func (p *listenerOptsPlugin) Name() string {
	return "listenerpolicies"
}

// called 1 time for each listener
func (p *listenerOptsPluginGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
	policy, ok := pCtx.Policy.(*listenerOptsPlugin)
	if !ok {
		// todo: allow returning error
		return
	}

	if policy.spec.PerConnectionBufferLimitBytes > 0 {
		out.PerConnectionBufferLimitBytes = wrapperspb.UInt32(policy.spec.PerConnectionBufferLimitBytes)
	}

	return
}

func (p *listenerOptsPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *listenerOptsPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	return nil
}

func (p *listenerOptsPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from listener config must be disabled, so it doesnt impact other listeners.
func (p *listenerOptsPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	return nil, nil
}

func (p *listenerOptsPluginGwPass) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *listenerOptsPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *listenerOptsPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}
