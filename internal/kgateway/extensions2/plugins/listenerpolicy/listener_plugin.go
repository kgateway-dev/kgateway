package listenerpolicy

import (
	"context"
	"fmt"
	"time"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	proxy_protocol "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	kgwwellknown "github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

var logger = logging.New("plugin/listenerpolicy")

type listenerPolicy struct {
	ct                            time.Time
	proxyProtocol                 *anypb.Any
	perConnectionBufferLimitBytes *uint32
}

func (d *listenerPolicy) CreationTime() time.Time {
	return d.ct
}

func (d *listenerPolicy) Equals(in any) bool {
	d2, ok := in.(*listenerPolicy)
	if !ok {
		return false
	}

	if d.ct != d2.ct {
		return false
	}

	if !proto.Equal(d.proxyProtocol, d2.proxyProtocol) {
		return false
	}

	if !cmputils.PointerValsEqual(d.perConnectionBufferLimitBytes, d2.perConnectionBufferLimitBytes) {
		return false
	}

	return true
}

func getPolicyStatusFn(
	cl kclient.Client[*v1alpha1.ListenerPolicy],
) sdk.GetPolicyStatusFn {
	return func(ctx context.Context, nn types.NamespacedName) (gwv1.PolicyStatus, error) {
		res := cl.Get(nn.Name, nn.Namespace)
		if res == nil {
			return gwv1.PolicyStatus{}, sdk.ErrNotFound
		}
		return res.Status, nil
	}
}

func patchPolicyStatusFn(
	cl kclient.Client[*v1alpha1.ListenerPolicy],
) sdk.PatchPolicyStatusFn {
	return func(ctx context.Context, nn types.NamespacedName, policyStatus gwv1.PolicyStatus) error {
		cur := cl.Get(nn.Name, nn.Namespace)
		if cur == nil {
			return sdk.ErrNotFound
		}
		if _, err := cl.UpdateStatus(&v1alpha1.ListenerPolicy{
			ObjectMeta: sdk.CloneObjectMetaForStatus(cur.ObjectMeta),
			Status:     policyStatus,
		}); err != nil {
			if errors.IsConflict(err) {
				logger.Debug("error updating stale status", "ref", nn, "error", err)
				return nil // let the conflicting Status update trigger a KRT event to requeue the updated object
			}
			return fmt.Errorf("error updating status for ListenerPolicy %s: %w", nn.String(), err)
		}
		return nil
	}
}

var _ ir.PolicyIR = &listenerPolicy{}

type listenerPolicyPluginGwPass struct {
	ir.UnimplementedProxyTranslationPass
	reporter reporter.Reporter
}

var _ ir.ProxyTranslationPass = &listenerPolicyPluginGwPass{}

func NewPlugin(ctx context.Context, commoncol *collections.CommonCollections) sdk.Plugin {
	cli := kclient.NewFilteredDelayed[*v1alpha1.ListenerPolicy](
		commoncol.Client,
		kgwwellknown.ListenerPolicyGVR,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	)
	col := krt.WrapClient(cli, commoncol.KrtOpts.ToOptions("ListenerPolicy")...)
	gk := kgwwellknown.ListenerPolicyGVK.GroupKind()

	policyStatusMarker, policyCol := krt.NewStatusCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.ListenerPolicy) (*krtcollections.StatusMarker, *ir.PolicyWrapper) {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: i.Namespace,
			Name:      i.Name,
		}

		// Create status marker if existing status has kgateway controller
		var statusMarker *krtcollections.StatusMarker
		for _, ancestor := range i.Status.Ancestors {
			if string(ancestor.ControllerName) == commoncol.ControllerName {
				statusMarker = &krtcollections.StatusMarker{}
				break
			}
		}
		var perConnectionBufferLimitBytes *uint32
		if i.Spec.PerConnectionBufferLimitBytes != nil {
			perConnectionBufferLimitBytes = ptr.To(uint32(*i.Spec.PerConnectionBufferLimitBytes)) //nolint:gosec // G115: kubebuilder validation ensures 0 <= value <= 2147483647, safe for uint32
		}

		pol := &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       i,
			PolicyIR: &listenerPolicy{
				ct:                            i.CreationTimestamp.Time,
				proxyProtocol:                 convertProxyProtocolConfig(objSrc, i.Spec.ProxyProtocol),
				perConnectionBufferLimitBytes: perConnectionBufferLimitBytes,
			},
			TargetRefs: pluginsdkutils.TargetRefsToPolicyRefs(i.Spec.TargetRefs, i.Spec.TargetSelectors),
			Errors:     []error{},
		}

		return statusMarker, pol
	}, commoncol.KrtOpts.ToOptions("ListenerPolicyWrapper")...)

	// processMarkers for policies that have existing status but no current report
	processMarkers := func(kctx krt.HandlerContext, reportMap *reports.ReportMap) {
		objStatus := krt.Fetch(kctx, policyStatusMarker)
		for _, status := range objStatus {
			policyKey := reporter.PolicyKey{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: status.Obj.GetNamespace(),
				Name:      status.Obj.GetName(),
			}

			// Add empty status to clear stale status for policies with no valid targets
			if reportMap.Policies[policyKey] == nil {
				rp := reports.NewReporter(reportMap)
				// create empty policy report entry with no ancestor refs
				rp.Policy(policyKey, 0)
			}
		}
	}
	return sdk.Plugin{
		ExtraHasSynced: col.HasSynced,
		ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			kgwwellknown.ListenerPolicyGVK.GroupKind(): {
				NewGatewayTranslationPass:       NewGatewayTranslationPass,
				Policies:                        policyCol,
				ProcessPolicyStaleStatusMarkers: processMarkers,
				GetPolicyStatus:                 getPolicyStatusFn(cli),
				PatchPolicyStatus:               patchPolicyStatusFn(cli),
			},
		},
	}
}

func NewGatewayTranslationPass(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass {
	return &listenerPolicyPluginGwPass{
		reporter: reporter,
	}
}

func (p *listenerPolicyPluginGwPass) Name() string {
	return "listenerpolicy"
}

func (p *listenerPolicyPluginGwPass) ApplyListenerPlugin(
	pCtx *ir.ListenerContext,
	out *envoylistenerv3.Listener,
) {
	logger.Debug("applying to listener", "listener", out.Name, "policyType", fmt.Sprintf("%T", pCtx.Policy))
	pol, ok := pCtx.Policy.(*listenerPolicy)
	if !ok || pol == nil {
		logger.Warn("policy is not listenerPolicy type or is nil", "ok", ok, "pol", pol)
		return
	}

	logger.Debug("listenerPolicy found", "proxy_protocol", pol.proxyProtocol, "per_connection_buffer_limit_bytes", pol.perConnectionBufferLimitBytes)
	// Add proxy protocol listener filter if configured
	if pol.proxyProtocol != nil {
		p.applyProxyProtocol(out, pol.proxyProtocol)
	}
	// Set per connection buffer limit if configured
	if pol.perConnectionBufferLimitBytes != nil {
		out.PerConnectionBufferLimitBytes = &wrapperspb.UInt32Value{Value: *pol.perConnectionBufferLimitBytes}
	}
}

func convertProxyProtocolConfig(objSrc ir.ObjectSource, config *v1alpha1.ProxyProtocolConfig) *anypb.Any {
	if config == nil {
		return nil
	}
	// Create the proxy protocol configuration
	proxyProtocolConfig := &proxy_protocol.ProxyProtocol{
		StatPrefix: fmt.Sprintf("%s_%s", objSrc.Namespace, objSrc.Name),
	}

	// Marshal to Any
	proxyProtocolAny, err := utils.MessageToAny(proxyProtocolConfig)
	if err != nil {
		logger.Error("failed to marshal proxy protocol config",
			"error", err)
	}
	return proxyProtocolAny
}

func (p *listenerPolicyPluginGwPass) applyProxyProtocol(
	out *envoylistenerv3.Listener,
	proxyProtocolAny *anypb.Any,
) {
	// Check if proxy protocol filter already exists (e.g., from sandwich plugin)
	for _, lf := range out.GetListenerFilters() {
		if lf.Name == wellknown.ProxyProtocol {
			logger.Warn("proxy protocol listener filter already exists, skipping",
				"listener", out.Name,
				"note", "this may conflict with waypoint sandwich mode")
			return
		}
	}

	// Create the listener filter
	listenerFilter := &envoylistenerv3.ListenerFilter{
		Name: wellknown.ProxyProtocol,
		ConfigType: &envoylistenerv3.ListenerFilter_TypedConfig{
			TypedConfig: proxyProtocolAny,
		},
	}

	// Prepend the proxy protocol filter to the beginning of the listener filters
	// This ensures it runs before TLS inspector and other filters
	// Given that we currently only have 2 listener filters, this one and tls_inspector, this should be sufficient
	// if we get more listener filters in the future we may need a more robust ordering mechanism
	out.ListenerFilters = append([]*envoylistenerv3.ListenerFilter{listenerFilter}, out.GetListenerFilters()...)

	logger.Debug("added proxy protocol listener filter", "listener", out.Name)
}
