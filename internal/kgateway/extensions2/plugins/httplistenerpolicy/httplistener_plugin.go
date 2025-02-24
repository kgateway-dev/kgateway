package httplistenerpolicy

import (
	"context"
	"fmt"
	"slices"
	"time"

	envoyaccesslog "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

type httpListenerPolicy struct {
	ct        time.Time
	compress  bool
	accessLog []*envoyaccesslog.AccessLog
}

func (d *httpListenerPolicy) CreationTime() time.Time {
	return d.ct
}

func (d *httpListenerPolicy) Equals(in any) bool {
	d2, ok := in.(*httpListenerPolicy)
	if !ok {
		return false
	}

	// Check the TargetRef and Compress fields
	if d.compress != d2.compress {
		return false
	}

	// Check the AccessLog slice
	if !slices.EqualFunc(d.accessLog, d2.accessLog, func(log *envoyaccesslog.AccessLog, log2 *envoyaccesslog.AccessLog) bool {
		return proto.Equal(log, log2)
	}) {
		return false
	}

	return true
}

type httpListenerPolicyPluginGwPass struct {
}

func (p *httpListenerPolicyPluginGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
	// no op
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionplug.Plugin {
	// need upstreams for backend refs
	col := krtutil.SetupCollectionDynamic[v1alpha1.HTTPListenerPolicy](
		ctx,
		commoncol.Client,
		v1alpha1.SchemeGroupVersion.WithResource("httplistenerpolicies"),
		commoncol.KrtOpts.ToOptions("HTTPListenerPolicy")...,
	)
	gk := v1alpha1.HTTPListenerPolicyGVK.GroupKind()
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.HTTPListenerPolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: i.Namespace,
			Name:      i.Name,
		}

		errors := []error{}
		accessLog, err := convertAccessLogConfig(ctx, i, commoncol, krtctx, objSrc)
		if err != nil {
			contextutils.LoggerFrom(ctx).Error(err)
			errors = append(errors, err)
		}

		var pol = &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       i,
			PolicyIR: &httpListenerPolicy{
				ct:        i.CreationTimestamp.Time,
				compress:  i.Spec.Compress,
				accessLog: accessLog,
			},
			TargetRefs: convert(i.Spec.TargetRef),
			Errors:     errors,
		}

		return pol
	})

	return extensionplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			v1alpha1.HTTPListenerPolicyGVK.GroupKind(): {
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
	return &httpListenerPolicyPluginGwPass{}
}
func (p *httpListenerPolicyPluginGwPass) Name() string {
	return "httplistenerpolicies"
}

func (p *httpListenerPolicyPluginGwPass) ApplyHCM(
	ctx context.Context,
	pCtx *ir.HcmContext,
	out *envoy_hcm.HttpConnectionManager) error {
	policy, ok := pCtx.Policy.(*httpListenerPolicy)
	if !ok {
		return fmt.Errorf("internal error: expected httplistener policy, got %T", pCtx.Policy)
	}

	// translate access logging configuration
	out.AccessLog = append(out.GetAccessLog(), policy.accessLog...)
	return nil
}

func (p *httpListenerPolicyPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *httpListenerPolicyPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	return nil
}

func (p *httpListenerPolicyPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from listener config must be disabled, so it doesnt impact other listeners.
func (p *httpListenerPolicyPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	return nil, nil
}

func (p *httpListenerPolicyPluginGwPass) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *httpListenerPolicyPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *httpListenerPolicyPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}
