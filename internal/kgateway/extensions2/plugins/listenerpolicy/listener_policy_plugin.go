package listenerpolicy

import (
	"context"
	"time"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/google/go-cmp/cmp"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
)

type listenerPolicy struct {
	ct   time.Time
	spec v1alpha1.ListenerPolicySpec
}

func (d *listenerPolicy) CreationTime() time.Time {
	return d.ct
}

func (d *listenerPolicy) Equals(in any) bool {
	d2, ok := in.(*listenerPolicy)
	if !ok {
		return false
	}
	return cmp.Equal(d.spec, d2.spec)
}

type listenerPolicyPluginGwPass struct {
	ir.UnimplementedProxyTranslationPass
}

func (p *listenerPolicyPluginGwPass) ApplyForBackend(ctx context.Context, pCtx *ir.RouteBackendContext, in ir.HttpBackend, out *envoy_config_route_v3.Route) error {
	// no op
	return nil
}

func registerTypes(ourCli versioned.Interface) {
	skubeclient.Register[*v1alpha1.ListenerPolicy](
		wellknown.ListenerPolicyGVR,
		wellknown.ListenerPolicyGVK,
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().ListenerPolicies(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().ListenerPolicies(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionplug.Plugin {
	registerTypes(commoncol.OurClient)

	col := krt.WrapClient(kclient.New[*v1alpha1.ListenerPolicy](commoncol.Client), commoncol.KrtOpts.ToOptions("ListenerPolicy")...)
	gk := wellknown.ListenerPolicyGVK.GroupKind()
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.ListenerPolicy) *ir.PolicyWrapper {
		pol := &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Policy:     i,
			PolicyIR:   &listenerPolicy{ct: i.CreationTimestamp.Time, spec: i.Spec},
			TargetRefs: convert(i.Spec.TargetRefs),
		}
		return pol
	})

	return extensionplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			wellknown.ListenerPolicyGVK.GroupKind(): {
				// AttachmentPoints: []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				NewGatewayTranslationPass: NewGatewayTranslationPass,
				Policies:                  policyCol,
			},
		},
	}
}

func convert(targetRefs []v1alpha1.LocalPolicyTargetReference) []ir.PolicyRef {
	refs := make([]ir.PolicyRef, 0, len(targetRefs))
	for _, targetRef := range targetRefs {
		refs = append(refs, ir.PolicyRef{
			Kind:  string(targetRef.Kind),
			Name:  string(targetRef.Name),
			Group: string(targetRef.Group),
		})
	}
	return refs
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &listenerPolicyPluginGwPass{}
}

func (p *listenerPolicy) Name() string {
	return "listenerpolicies"
}

// called 1 time for each listener
func (p *listenerPolicyPluginGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
	policy, ok := pCtx.Policy.(*listenerPolicy)
	if !ok {
		// todo: allow returning error
		return
	}

	if policy.spec.PerConnectionBufferLimitBytes > 0 {
		out.PerConnectionBufferLimitBytes = wrapperspb.UInt32(policy.spec.PerConnectionBufferLimitBytes)
	}

	return
}

func (p *listenerPolicyPluginGwPass) ApplyHCM(
	ctx context.Context,
	pCtx *ir.HcmContext,
	out *envoy_hcm.HttpConnectionManager,
) error {
	// no-op, hcm config is handled in http listener plugin
	return nil
}

func (p *listenerPolicyPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *listenerPolicyPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	return nil
}

func (p *listenerPolicyPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from listener config must be disabled, so it doesnt impact other listeners.
func (p *listenerPolicyPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	return nil, nil
}

func (p *listenerPolicyPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *listenerPolicyPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}
