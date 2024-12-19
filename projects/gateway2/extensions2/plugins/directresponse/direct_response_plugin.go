package directresponse

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/common"
	extensionplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/pkg/client/clientset/versioned"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

type directResponse struct {
	ct   time.Time
	spec v1alpha1.DirectResponseSpec
}

// in case multiple policies attached to the same resouce, we sort by policy creation time.
func (d *directResponse) CreationTime() time.Time {
	return d.ct
}

func (d *directResponse) Equals(in any) bool {
	d2, ok := in.(*directResponse)
	if !ok {
		return false
	}
	return d.spec == d2.spec
}

type directResponseGwPass struct {
}

func registerTypes(ourCli versioned.Interface) {
	skubeclient.Register[*v1alpha1.DirectResponse](
		v1alpha1.DirectResponseGVK.GroupVersion().WithResource("directresponses"),
		v1alpha1.DirectResponseGVK,
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().DirectResponses(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().DirectResponses(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionplug.Plugin {

	registerTypes(commoncol.OurClient)

	col := krt.WrapClient(kclient.New[*v1alpha1.DirectResponse](commoncol.Client), commoncol.KrtOpts.ToOptions("DirectResponse")...)

	gk := v1alpha1.DirectResponseGVK.GroupKind()
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.DirectResponse) *ir.PolicyWrapper {
		var pol = &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Policy:   i,
			PolicyIR: &directResponse{ct: i.CreationTimestamp.Time, spec: i.Spec},
			// no target refs for direct response
		}
		return pol
	})

	return extensionplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionplug.PolicyPlugin{
			v1alpha1.DirectResponseGVK.GroupKind(): {
				Name: "directresponse",
				//	AttachmentPoints: []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				Policies: policyCol,
				//				AttachmentPoints:          []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				NewGatewayTranslationPass: NewGatewayTranslationPass,
			},
		},
	}
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &directResponseGwPass{}
}

// called 1 time for each listener
func (p *directResponseGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *directResponseGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *directResponseGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	dr, ok := pCtx.Policy.(*directResponse)
	if !ok {
		return fmt.Errorf("internal error: expected *directResponse, got %T", pCtx.Policy)
	}
	// at this point, we have a valid DR reference that we should apply to the route.
	if outputRoute.GetAction() != nil {
		// the output route already has an action, which is incompatible with the DirectResponse,
		// so we'll return an error. note: the direct response plugin runs after other route plugins
		// that modify the output route (e.g. the redirect plugin), so this should be a rare case.
		errMsg := fmt.Sprintf("DirectResponse cannot be applied to route with existing action: %T", outputRoute.GetAction())

		outputRoute.Action = &envoy_config_route_v3.Route_DirectResponse{
			DirectResponse: &envoy_config_route_v3.DirectResponseAction{
				Status: http.StatusInternalServerError,
			},
		}
		return fmt.Errorf(errMsg)
	}

	outputRoute.Action = &envoy_config_route_v3.Route_DirectResponse{
		DirectResponse: &envoy_config_route_v3.DirectResponseAction{
			Status: dr.spec.StatusCode,
			Body: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineString{
					InlineString: dr.spec.Body,
				},
			},
		},
	}
	return nil
}

func (p *directResponseGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	return ir.ErrNotAttachable
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *directResponseGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	return nil, nil
}

func (p *directResponseGwPass) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *directResponseGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *directResponseGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}
