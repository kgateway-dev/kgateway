package httplistenerpolicy

import (
	"context"
	"fmt"
	"slices"
	"time"

	envoyaccesslog "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

var logger = logging.New("plugin/httplistenerpolicy")

type httpListenerPolicy struct {
	ct             time.Time
	accessLog      []*envoyaccesslog.AccessLog
	upgradeConfigs []*envoy_hcm.HttpConnectionManager_UpgradeConfig
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

	// Check upgrade configs
	if !slices.EqualFunc(d.upgradeConfigs, d2.upgradeConfigs, func(cfg, cfg2 *envoy_hcm.HttpConnectionManager_UpgradeConfig) bool {
		return proto.Equal(cfg, cfg2)
	}) {
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

		upgradeConfigs := convertUpgradeConfig(i)

		pol := &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       i,
			PolicyIR: &httpListenerPolicy{
				ct:             i.CreationTimestamp.Time,
				accessLog:      accessLog,
				upgradeConfigs: upgradeConfigs,
			},
			TargetRefs: pluginutils.TargetRefsToPolicyRefs(i.Spec.TargetRefs, i.Spec.TargetSelectors),
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

	// translate upgrade configuration
	if policy.upgradeConfigs != nil {
		out.UpgradeConfigs = append(out.GetUpgradeConfigs(), policy.upgradeConfigs...)
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
