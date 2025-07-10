package httplistenerpolicy

import (
	"context"
	"fmt"
	"slices"
	"time"

	envoyaccesslog "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var logger = logging.New("plugin/httplistenerpolicy")

type httpListenerPolicy struct {
	ct                         time.Time
	accessLog                  []*envoyaccesslog.AccessLog
	tracing                    *envoy_hcm.HttpConnectionManager_Tracing
	upgradeConfigs             []*envoy_hcm.HttpConnectionManager_UpgradeConfig
	useRemoteAddress           *bool
	xffNumTrustedHops          *uint32
	serverHeaderTransformation *envoy_hcm.HttpConnectionManager_ServerHeaderTransformation
	streamIdleTimeout          *time.Duration
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
	if !slices.EqualFunc(d.accessLog, d2.accessLog, func(log *envoyaccesslog.AccessLog, log2 *envoyaccesslog.AccessLog) bool {
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
	if d.useRemoteAddress == nil && d2.useRemoteAddress != nil {
		return false
	}
	if d.useRemoteAddress != nil && d2.useRemoteAddress == nil {
		return false
	}
	if d.useRemoteAddress != nil && d2.useRemoteAddress != nil && *d.useRemoteAddress != *d2.useRemoteAddress {
		return false
	}

	// Check xffNumTrustedHops
	if d.xffNumTrustedHops == nil && d2.xffNumTrustedHops != nil {
		return false
	}
	if d.xffNumTrustedHops != nil && d2.xffNumTrustedHops == nil {
		return false
	}
	if d.xffNumTrustedHops != nil && d2.xffNumTrustedHops != nil && *d.xffNumTrustedHops != *d2.xffNumTrustedHops {
		return false
	}

	// Check serverHeaderTransformation
	if d.serverHeaderTransformation != d2.serverHeaderTransformation {
		return false
	}

	// Check streamIdleTimeout
	if d.streamIdleTimeout == nil && d2.streamIdleTimeout != nil {
		return false
	}
	if d.streamIdleTimeout != nil && d2.streamIdleTimeout == nil {
		return false
	}
	if d.streamIdleTimeout != nil && d2.streamIdleTimeout != nil && *d.streamIdleTimeout != *d2.streamIdleTimeout {
		return false
	}

	return true
}

type httpListenerPolicyPluginGwPass struct {
	ir.UnimplementedProxyTranslationPass
	reporter reports.Reporter
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

	return nil
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
