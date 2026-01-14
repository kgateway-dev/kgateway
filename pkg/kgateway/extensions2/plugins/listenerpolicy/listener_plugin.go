package listenerpolicy

import (
	"context"
	"fmt"
	"maps"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	healthcheckv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/health_check/v3"
	proxy_protocol "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	preserve_case_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/header_formatters/preserve_case/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	kgwwellknown "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

var logger = logging.New("plugin/listenerpolicy")

type ListenerPolicyIR struct {
	ct            time.Time
	defaultPolicy listenerPolicy
	perPortPolicy map[uint32]listenerPolicy

	NoOrigin bool // +noKrtEquals reason: When set to true, suppress source reporting metadata from
	// ListenerPolicy specific fields that are irrelevant to the (now deprecated) HTTPListenerPolicy. Remove when HTTPListenerPolicy is removed.
}

type listenerPolicy struct {
	proxyProtocol                 *anypb.Any
	perConnectionBufferLimitBytes *uint32
	http                          *HttpListenerPolicyIr
}

func newListenerPolicy(
	krtctx krt.HandlerContext, commoncol *collections.CommonCollections,
	objSrc ir.ObjectSource, i *kgateway.ListenerConfig) (listenerPolicy, []error) {
	if i == nil {
		return listenerPolicy{}, nil
	}
	var perConnectionBufferLimitBytes *uint32
	if i.PerConnectionBufferLimitBytes != nil {
		perConnectionBufferLimitBytes = ptr.To(uint32(*i.PerConnectionBufferLimitBytes)) //nolint:gosec // G115: kubebuilder validation ensures 0 <= value <= 2147483647, safe for uint32
	}
	http, errs := NewHttpListenerPolicy(krtctx, commoncol, i.HTTPSettings, objSrc)

	return listenerPolicy{
		proxyProtocol:                 convertProxyProtocolConfig(objSrc, i.ProxyProtocol),
		perConnectionBufferLimitBytes: perConnectionBufferLimitBytes,
		http:                          http,
	}, errs
}

func (d *ListenerPolicyIR) CreationTime() time.Time {
	return d.ct
}

func (d *ListenerPolicyIR) Equals(in any) bool {
	d2, ok := in.(*ListenerPolicyIR)
	if !ok {
		return false
	}

	if d.ct != d2.ct {
		return false
	}
	if !d.defaultPolicy.Equals(d2.defaultPolicy) {
		return false
	}
	if !maps.EqualFunc(d.perPortPolicy, d2.perPortPolicy, func(a, b listenerPolicy) bool {
		return a.Equals(b)
	}) {
		return false
	}
	return true
}

func (d listenerPolicy) Equals(d2 listenerPolicy) bool {
	if !proto.Equal(d.proxyProtocol, d2.proxyProtocol) {
		return false
	}

	if !cmputils.PointerValsEqual(d.perConnectionBufferLimitBytes, d2.perConnectionBufferLimitBytes) {
		return false
	}

	if (d.http == nil) != (d2.http == nil) {
		return false
	}
	if d.http != nil && !d.http.Equals(d2.http) {
		return false
	}

	return true
}

func getPolicyStatusFn(
	cl kclient.Client[*kgateway.ListenerPolicy],
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
	cl kclient.Client[*kgateway.ListenerPolicy],
) sdk.PatchPolicyStatusFn {
	return func(ctx context.Context, nn types.NamespacedName, policyStatus gwv1.PolicyStatus) error {
		cur := cl.Get(nn.Name, nn.Namespace)
		if cur == nil {
			return sdk.ErrNotFound
		}
		if _, err := cl.UpdateStatus(&kgateway.ListenerPolicy{
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

var _ ir.PolicyIR = &ListenerPolicyIR{}

type listenerPolicyPluginGwPass struct {
	ir.UnimplementedProxyTranslationPass
	reporter reporter.Reporter

	healthCheckPolicy map[uint32]*healthcheckv3.HealthCheck
}

var _ ir.ProxyTranslationPass = &listenerPolicyPluginGwPass{}

func NewListenerPolicyIR(
	krtctx krt.HandlerContext,
	commoncol *collections.CommonCollections,
	ct time.Time,
	spec *kgateway.ListenerPolicySpec,
	objSrc ir.ObjectSource,
) (*ListenerPolicyIR, []error) {
	if spec == nil {
		return nil, nil
	}
	errs := []error{}
	perPort := map[uint32]listenerPolicy{}
	for _, portConfig := range spec.PerPort {
		pol, errs2 := newListenerPolicy(krtctx, commoncol, objSrc, &portConfig.Listener)
		perPort[uint32(portConfig.Port)] = pol //nolint:gosec // G115: we have CEL validation that this is at least 1.
		errs = append(errs, errs2...)
	}
	defaultPolicy, errs2 := newListenerPolicy(krtctx, commoncol, objSrc, spec.Default)
	errs = append(errs, errs2...)
	return &ListenerPolicyIR{
		ct:            ct,
		defaultPolicy: defaultPolicy,
		perPortPolicy: perPort,
	}, errs
}

func NewPlugin(ctx context.Context, commoncol *collections.CommonCollections) sdk.Plugin {
	cli := kclient.NewFilteredDelayed[*kgateway.ListenerPolicy](
		commoncol.Client,
		kgwwellknown.ListenerPolicyGVR,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	)
	col := krt.WrapClient(cli, commoncol.KrtOpts.ToOptions("ListenerPolicy")...)
	gk := kgwwellknown.ListenerPolicyGVK.GroupKind()

	policyStatusMarker, policyCol := krt.NewStatusCollection(col, func(krtctx krt.HandlerContext, i *kgateway.ListenerPolicy) (*krtcollections.StatusMarker, *ir.PolicyWrapper) {
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

		polIr, errs := NewListenerPolicyIR(krtctx, commoncol, i.CreationTimestamp.Time, &i.Spec, objSrc)
		pol := &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       i,
			PolicyIR:     polIr,
			TargetRefs:   pluginsdkutils.TargetRefsToPolicyRefs(i.Spec.TargetRefs, i.Spec.TargetSelectors),
			Errors:       errs,
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
				MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
					return policy.MergePolicies(pols, MergePolicies, "" /*no merge settings*/)
				},
			},
		},
	}
}

func NewGatewayTranslationPass(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass {
	return &listenerPolicyPluginGwPass{
		reporter:          reporter,
		healthCheckPolicy: map[uint32]*healthcheckv3.HealthCheck{},
	}
}

func (p *listenerPolicyPluginGwPass) Name() string {
	return "listenerpolicy"
}
func (p *listenerPolicyPluginGwPass) getPolicy(policy ir.PolicyIR, port uint32) listenerPolicy {
	pol, ok := policy.(*ListenerPolicyIR)
	if !ok || pol == nil {
		logger.Warn("policy is not listenerPolicy type or is nil", "ok", ok, "pol", pol)
		return listenerPolicy{}
	}

	if perPortCfg, found := pol.perPortPolicy[port]; found {
		return perPortCfg
	}
	return pol.defaultPolicy
}

func (p *listenerPolicyPluginGwPass) ApplyListenerPlugin(
	pCtx *ir.ListenerContext,
	out *envoylistenerv3.Listener,
) {
	logger.Debug("applying to listener", "listener", out.Name, "policyType", fmt.Sprintf("%T", pCtx.Policy))
	cfg := p.getPolicy(pCtx.Policy, pCtx.Port)

	logger.Debug("listenerPolicy found", "proxy_protocol", cfg.proxyProtocol, "per_connection_buffer_limit_bytes", cfg.perConnectionBufferLimitBytes)
	// Add proxy protocol listener filter if configured
	if cfg.proxyProtocol != nil {
		p.applyProxyProtocol(out, cfg.proxyProtocol)
	}
	// Set per connection buffer limit if configured
	if cfg.perConnectionBufferLimitBytes != nil {
		out.PerConnectionBufferLimitBytes = &wrapperspb.UInt32Value{Value: *cfg.perConnectionBufferLimitBytes}
	}
	if http := cfg.http; http != nil {
		p.healthCheckPolicy[pCtx.Port] = http.healthCheckPolicy
	}
}

func (p *listenerPolicyPluginGwPass) HttpFilters(hCtx ir.HttpFiltersContext, fc ir.FilterChainCommon) ([]filters.StagedHttpFilter, error) {
	healthCheckPolicy := p.healthCheckPolicy[hCtx.ListenerPort]
	if healthCheckPolicy == nil {
		return nil, nil
	}

	// Add the health check filter after the authz filter but before the rate limit filter
	// This allows the health check filter to be secured by authz if needed, but ensures it won't be rate limited
	stagedFilter, err := filters.NewStagedFilter(
		"envoy.filters.http.health_check",
		healthCheckPolicy,
		filters.AfterStage(filters.AuthZStage),
	)
	if err != nil {
		return nil, err
	}

	return []filters.StagedHttpFilter{stagedFilter}, nil
}

func (p *listenerPolicyPluginGwPass) ApplyHCM(
	pCtx *ir.HcmContext,
	out *envoy_hcm.HttpConnectionManager,
) error {
	cfg := p.getPolicy(pCtx.Policy, pCtx.ListenerPort)
	policy := cfg.http
	if policy == nil {
		return nil
	}

	// translate access logging configuration
	accessLogs, err := generateAccessLogConfig(pCtx, policy.accessLogPolicies, policy.accessLogConfig)
	if err != nil {
		return err
	}
	out.AccessLog = append(out.GetAccessLog(), accessLogs...)

	// translate tracing configuration
	updateTracingConfig(pCtx, policy.tracingProvider, policy.tracingConfig)
	out.Tracing = policy.tracingConfig

	// translate upgrade configuration
	if policy.upgradeConfigs != nil {
		out.UpgradeConfigs = append(out.GetUpgradeConfigs(), policy.upgradeConfigs...)
	}

	// translate useRemoteAddress
	if policy.useRemoteAddress != nil {
		out.UseRemoteAddress = wrapperspb.Bool(*policy.useRemoteAddress)
	}
	if policy.preserveExternalRequestId != nil {
		out.PreserveExternalRequestId = *policy.preserveExternalRequestId
	}
	if policy.generateRequestId != nil {
		out.GenerateRequestId = wrapperspb.Bool(*policy.generateRequestId)
	}

	// translate xffNumTrustedHops
	if policy.xffNumTrustedHops != nil {
		out.XffNumTrustedHops = *policy.xffNumTrustedHops
	}

	// translate serverHeaderTransformation
	if policy.serverHeaderTransformation != nil {
		out.ServerHeaderTransformation = *policy.serverHeaderTransformation
	}

	// translate streamIdleTimeout
	if policy.streamIdleTimeout != nil {
		out.StreamIdleTimeout = durationpb.New(*policy.streamIdleTimeout)
	}
	// early request header modifier
	if len(policy.earlyHeaderMutationExtensions) != 0 {
		out.EarlyHeaderMutationExtensions = append(out.EarlyHeaderMutationExtensions, policy.earlyHeaderMutationExtensions...)
	}

	// translate idleTimeout
	if policy.idleTimeout != nil {
		if out.CommonHttpProtocolOptions == nil {
			out.CommonHttpProtocolOptions = &envoycorev3.HttpProtocolOptions{}
		}
		out.GetCommonHttpProtocolOptions().IdleTimeout = durationpb.New(*policy.idleTimeout)
	}

	if policy.preserveHttp1HeaderCase != nil && *policy.preserveHttp1HeaderCase {
		if out.HttpProtocolOptions == nil {
			out.HttpProtocolOptions = &envoycorev3.Http1ProtocolOptions{}
		}
		preservecaseAny, err := utils.MessageToAny(&preserve_case_v3.PreserveCaseFormatterConfig{})
		if err != nil {
			// shouldn't happen
			logger.Error("error translating preserveHttp1HeaderCase", "error", err)
			return nil
		}
		out.GetHttpProtocolOptions().HeaderKeyFormat = &envoycorev3.Http1ProtocolOptions_HeaderKeyFormat{
			HeaderFormat: &envoycorev3.Http1ProtocolOptions_HeaderKeyFormat_StatefulFormatter{
				StatefulFormatter: &envoycorev3.TypedExtensionConfig{
					Name:        "envoy.http.stateful_header_formatters.preserve_case",
					TypedConfig: preservecaseAny,
				},
			},
		}
	}

	if policy.acceptHttp10 != nil && *policy.acceptHttp10 {
		if out.HttpProtocolOptions == nil {
			out.HttpProtocolOptions = &envoycorev3.Http1ProtocolOptions{}
		}
		out.HttpProtocolOptions.AcceptHttp_10 = true
	}

	if policy.defaultHostForHttp10 != nil {
		if out.HttpProtocolOptions == nil {
			out.HttpProtocolOptions = &envoycorev3.Http1ProtocolOptions{}
		}
		out.HttpProtocolOptions.DefaultHostForHttp_10 = *policy.defaultHostForHttp10
	}

	// translate maxRequestHeadersKb
	if policy.maxRequestHeadersKb != nil {
		out.MaxRequestHeadersKb = wrapperspb.UInt32(*policy.maxRequestHeadersKb)
	}

	// translate uuidRequestIdConfig
	if policy.uuidRequestIdConfig != nil {
		requestIdExtensionAny, err := utils.MessageToAny(policy.uuidRequestIdConfig)
		if err != nil {
			logger.Error("error translating uuidRequestIdConfig", "error", err)
			return err
		}
		out.RequestIdExtension = &envoy_hcm.RequestIDExtension{
			TypedConfig: requestIdExtensionAny,
		}
	}

	return nil
}

func convertProxyProtocolConfig(objSrc ir.ObjectSource, config *kgateway.ProxyProtocolConfig) *anypb.Any {
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
