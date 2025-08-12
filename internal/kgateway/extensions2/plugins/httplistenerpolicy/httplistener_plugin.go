package httplistenerpolicy

import (
	"context"
	"fmt"
	"slices"
	"time"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	healthcheckv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/health_check/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	preserve_case_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/header_formatters/preserve_case/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

var logger = logging.New("plugin/httplistenerpolicy")

type httpListenerPolicy struct {
	ct                         time.Time
	accessLog                  []*envoyaccesslogv3.AccessLog
	tracing                    *envoy_hcm.HttpConnectionManager_Tracing
	upgradeConfigs             []*envoy_hcm.HttpConnectionManager_UpgradeConfig
	useRemoteAddress           *bool
	xffNumTrustedHops          *uint32
	serverHeaderTransformation *envoy_hcm.HttpConnectionManager_ServerHeaderTransformation
	streamIdleTimeout          *time.Duration
	healthCheckPolicy          *healthcheckv3.HealthCheck
	preserveHttp1HeaderCase    *bool
}

func (d *httpListenerPolicy) CreationTime() time.Time {
	return d.ct
}

func (d *httpListenerPolicy) Equals(in any) bool {
	d2, ok := in.(*httpListenerPolicy)
	if !ok {
		return false
	}

	// Check the AccessLog slice
	if !slices.EqualFunc(d.accessLog, d2.accessLog, func(log *envoyaccesslogv3.AccessLog, log2 *envoyaccesslogv3.AccessLog) bool {
		return proto.Equal(log, log2)
	}) {
		return false
	}

	// Check tracing
	if !proto.Equal(d.tracing, d2.tracing) {
		return false
	}

	// Check upgrade configs
	if !slices.EqualFunc(d.upgradeConfigs, d2.upgradeConfigs, func(cfg, cfg2 *envoy_hcm.HttpConnectionManager_UpgradeConfig) bool {
		return proto.Equal(cfg, cfg2)
	}) {
		return false
	}

	// Check useRemoteAddress
	if !cmputils.PointerValsEqual(d.useRemoteAddress, d2.useRemoteAddress) {
		return false
	}

	// Check xffNumTrustedHops
	if !cmputils.PointerValsEqual(d.xffNumTrustedHops, d2.xffNumTrustedHops) {
		return false
	}

	// Check serverHeaderTransformation
	if d.serverHeaderTransformation != d2.serverHeaderTransformation {
		return false
	}

	// Check streamIdleTimeout
	if !cmputils.PointerValsEqual(d.streamIdleTimeout, d2.streamIdleTimeout) {
		return false
	}

	// Check healthCheckPolicy
	if d.healthCheckPolicy == nil && d2.healthCheckPolicy != nil {
		return false
	}
	if d.healthCheckPolicy != nil && d2.healthCheckPolicy == nil {
		return false
	}
	if d.healthCheckPolicy != nil && d2.healthCheckPolicy != nil && !proto.Equal(d.healthCheckPolicy, d2.healthCheckPolicy) {
		return false
	}

	// Check healthCheckPolicy
	if !proto.Equal(d.healthCheckPolicy, d2.healthCheckPolicy) {
		return false
	}

	if !cmputils.PointerValsEqual(d.preserveHttp1HeaderCase, d2.preserveHttp1HeaderCase) {
		return false
	}

	return true
}

type httpListenerPolicyPluginGwPass struct {
	ir.UnimplementedProxyTranslationPass
	reporter reports.Reporter

	healthCheckPolicy *healthcheckv3.HealthCheck
}

var _ ir.ProxyTranslationPass = &httpListenerPolicyPluginGwPass{}

func registerTypes(ourCli versioned.Interface) {
	skubeclient.Register[*v1alpha1.HTTPListenerPolicy](
		wellknown.HTTPListenerPolicyGVR,
		wellknown.HTTPListenerPolicyGVK,
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().HTTPListenerPolicies(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().HTTPListenerPolicies(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes(commoncol.OurClient)

	col := krt.WrapClient(kclient.NewFiltered[*v1alpha1.HTTPListenerPolicy](
		commoncol.Client,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	), commoncol.KrtOpts.ToOptions("HTTPListenerPolicy")...)
	gk := wellknown.HTTPListenerPolicyGVK.GroupKind()
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.HTTPListenerPolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: i.Namespace,
			Name:      i.Name,
		}

		errs := []error{}
		accessLog, err := convertAccessLogConfig(ctx, i, commoncol, krtctx, objSrc)
		if err != nil {
			logger.Error("error translating access log", "error", err)
			errs = append(errs, err)
		}
		tracing, err := convertTracingConfig(ctx, i, commoncol, krtctx, objSrc)
		if err != nil {
			logger.Error("error translating tracing", "error", err)
			errs = append(errs, err)
		}

		upgradeConfigs := convertUpgradeConfig(i)
		serverHeaderTransformation := convertServerHeaderTransformation(i.Spec.ServerHeaderTransformation)

		// Convert streamIdleTimeout from metav1.Duration to time.Duration
		var streamIdleTimeout *time.Duration
		if i.Spec.StreamIdleTimeout != nil {
			duration := i.Spec.StreamIdleTimeout.Duration
			streamIdleTimeout = &duration
		}

		healthCheckPolicy := convertHealthCheckPolicy(i)

		pol := &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       i,
			PolicyIR: &httpListenerPolicy{
				ct:                         i.CreationTimestamp.Time,
				accessLog:                  accessLog,
				tracing:                    tracing,
				upgradeConfigs:             upgradeConfigs,
				useRemoteAddress:           i.Spec.UseRemoteAddress,
				xffNumTrustedHops:          i.Spec.XffNumTrustedHops,
				serverHeaderTransformation: serverHeaderTransformation,
				streamIdleTimeout:          streamIdleTimeout,
				healthCheckPolicy:          healthCheckPolicy,
				preserveHttp1HeaderCase:    i.Spec.PreserveHttp1HeaderCase,
			},
			TargetRefs: pluginsdkutils.TargetRefsToPolicyRefs(i.Spec.TargetRefs, i.Spec.TargetSelectors),
			Errors:     errs,
		}

		return pol
	})

	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			wellknown.HTTPListenerPolicyGVK.GroupKind(): {
				// AttachmentPoints: []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				NewGatewayTranslationPass: NewGatewayTranslationPass,
				Policies:                  policyCol,
				GetPolicyStatus:           getPolicyStatusFn(commoncol.CrudClient),
				PatchPolicyStatus:         patchPolicyStatusFn(commoncol.CrudClient),
				MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
					return policy.MergePolicies(pols, mergePolicies)
				},
			},
		},
	}
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx, reporter reports.Reporter) ir.ProxyTranslationPass {
	return &httpListenerPolicyPluginGwPass{
		reporter: reporter,
	}
}

func (p *httpListenerPolicyPluginGwPass) Name() string {
	return "httplistenerpolicies"
}

func (p *httpListenerPolicyPluginGwPass) ApplyHCM(
	ctx context.Context,
	pCtx *ir.HcmContext,
	out *envoy_hcm.HttpConnectionManager,
) error {
	policy, ok := pCtx.Policy.(*httpListenerPolicy)
	if !ok {
		return fmt.Errorf("internal error: expected httplistener policy, got %T", pCtx.Policy)
	}

	// translate access logging configuration
	out.AccessLog = append(out.GetAccessLog(), policy.accessLog...)

	// translate tracing configuration
	out.Tracing = policy.tracing

	// translate upgrade configuration
	if policy.upgradeConfigs != nil {
		out.UpgradeConfigs = append(out.GetUpgradeConfigs(), policy.upgradeConfigs...)
	}

	// translate useRemoteAddress
	if policy.useRemoteAddress != nil {
		out.UseRemoteAddress = wrapperspb.Bool(*policy.useRemoteAddress)
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

	if policy.preserveHttp1HeaderCase != nil && *policy.preserveHttp1HeaderCase {
		out.HttpProtocolOptions = &envoycorev3.Http1ProtocolOptions{}
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

	return nil
}

func (p *httpListenerPolicyPluginGwPass) HttpFilters(ctx context.Context, fc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	if p.healthCheckPolicy == nil {
		return nil, nil
	}

	// Add the health check filter after the authz filter but before the rate limit filter
	// This allows the health check filter to be secured by authz if needed, but ensures it won't be rate limited
	stagedFilter, err := plugins.NewStagedFilter(
		"envoy.filters.http.health_check",
		p.healthCheckPolicy,
		plugins.AfterStage(plugins.AuthZStage),
	)
	if err != nil {
		return nil, err
	}

	return []plugins.StagedHttpFilter{stagedFilter}, nil
}

func (p *httpListenerPolicyPluginGwPass) ApplyListenerPlugin(
	ctx context.Context,
	pCtx *ir.ListenerContext,
	out *envoylistenerv3.Listener,
) {
	policy, ok := pCtx.Policy.(*httpListenerPolicy)
	if !ok {
		return
	}

	p.healthCheckPolicy = policy.healthCheckPolicy
}

func convertUpgradeConfig(policy *v1alpha1.HTTPListenerPolicy) []*envoy_hcm.HttpConnectionManager_UpgradeConfig {
	if policy.Spec.UpgradeConfig == nil {
		return nil
	}

	configs := make([]*envoy_hcm.HttpConnectionManager_UpgradeConfig, 0, len(policy.Spec.UpgradeConfig.EnabledUpgrades))
	for _, upgradeType := range policy.Spec.UpgradeConfig.EnabledUpgrades {
		configs = append(configs, &envoy_hcm.HttpConnectionManager_UpgradeConfig{
			UpgradeType: upgradeType,
		})
	}
	return configs
}

func convertServerHeaderTransformation(transformation *v1alpha1.ServerHeaderTransformation) *envoy_hcm.HttpConnectionManager_ServerHeaderTransformation {
	if transformation == nil {
		return nil
	}

	switch *transformation {
	case v1alpha1.OverwriteServerHeaderTransformation:
		val := envoy_hcm.HttpConnectionManager_OVERWRITE
		return &val
	case v1alpha1.AppendIfAbsentServerHeaderTransformation:
		val := envoy_hcm.HttpConnectionManager_APPEND_IF_ABSENT
		return &val
	case v1alpha1.PassThroughServerHeaderTransformation:
		val := envoy_hcm.HttpConnectionManager_PASS_THROUGH
		return &val
	default:
		return nil
	}
}

func convertHealthCheckPolicy(policy *v1alpha1.HTTPListenerPolicy) *healthcheckv3.HealthCheck {
	if policy.Spec.HealthCheck != nil {
		return &healthcheckv3.HealthCheck{
			PassThroughMode: wrapperspb.Bool(false),
			Headers: []*envoyroutev3.HeaderMatcher{{
				Name: ":path",
				HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_StringMatch{
					StringMatch: &envoymatcherv3.StringMatcher{
						MatchPattern: &envoymatcherv3.StringMatcher_Exact{
							Exact: policy.Spec.HealthCheck.Path,
						},
					},
				},
			}},
		}
	}
	return nil
}
