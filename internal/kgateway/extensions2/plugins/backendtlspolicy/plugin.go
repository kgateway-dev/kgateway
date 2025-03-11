package backendtlspolicy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/solo-io/go-utils/contextutils"
	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	kgwellknown "github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

var backendTlsPolicyGvr = gwapiv1a3.SchemeGroupVersion.WithResource("backendtlspolicies")

type backendTlsPolicy struct {
	ct              time.Time
	transportSocket *envoy_config_core_v3.TransportSocket
}

var _ ir.PolicyIR = &backendTlsPolicy{}

func (d *backendTlsPolicy) CreationTime() time.Time {
	return d.ct
}

func (d *backendTlsPolicy) Equals(in any) bool {
	d2, ok := in.(*backendTlsPolicy)
	if !ok {
		return false
	}
	return proto.Equal(d.transportSocket, d2.transportSocket)
}

func registerTypes() {
	kubeclient.Register[*gwapiv1a3.BackendTLSPolicy](
		backendTlsPolicyGvr,
		kgwellknown.BackendTLSPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1alpha3().BackendTLSPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1alpha3().BackendTLSPolicies(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes()
	inf := kclient.NewDelayedInformer[*gwapiv1a3.BackendTLSPolicy](commoncol.Client, backendTlsPolicyGvr, kubetypes.StandardInformer, kclient.Filter{})
	col := krt.WrapClient(inf, commoncol.KrtOpts.ToOptions("BackendTLSPolicy")...)
	gk := kgwellknown.BackendTLSPolicyGVK.GroupKind()

	translate := buildTranslateFunc(ctx, commoncol.ConfigMaps)
	tlsPolicyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *gwapiv1a3.BackendTLSPolicy) *ir.PolicyWrapper {
		var pol = &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Policy:     i,
			PolicyIR:   translate(krtctx, i),
			TargetRefs: convertTargetRefs(i.Spec.TargetRefs),
		}
		return pol
	}, commoncol.KrtOpts.ToOptions("BackendTLSPolicyIRs")...)

	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			gk: {
				Name:           "BackendTLSPolicy",
				Policies:       tlsPolicyCol,
				ProcessBackend: ProcessBackend,
			},
		},
	}
}

func ProcessBackend(ctx context.Context, polir ir.PolicyIR, in ir.BackendObjectIR, out *clusterv3.Cluster) {
	tlsPol, ok := polir.(*backendTlsPolicy)
	if !ok {
		return
	}
	if tlsPol.transportSocket == nil {
		return
	}
	out.TransportSocket = tlsPol.transportSocket
}

func buildTranslateFunc(
	ctx context.Context,
	cfgmaps krt.Collection[*corev1.ConfigMap],
) func(krtctx krt.HandlerContext, i *gwapiv1a3.BackendTLSPolicy) *backendTlsPolicy {
	return func(krtctx krt.HandlerContext, policyCR *gwapiv1a3.BackendTLSPolicy) *backendTlsPolicy {
		spec := policyCR.Spec
		policyIr := backendTlsPolicy{
			ct: policyCR.CreationTimestamp.Time,
		}

		if len(spec.Validation.CACertificateRefs) == 0 {
			return &policyIr
		}

		certRef := spec.Validation.CACertificateRefs[0]
		nn := types.NamespacedName{
			Name:      string(certRef.Name),
			Namespace: policyCR.Namespace,
		}
		cfgmap := krt.FetchOne(krtctx, cfgmaps, krt.FilterObjectName(nn))
		if cfgmap == nil {
			contextutils.LoggerFrom(ctx).Error(errors.New(fmt.Sprintf("configmap %s not found", nn)))
			return &policyIr
		}

		tlsCfg, err := ResolveUpstreamSslConfig(*cfgmap, string(spec.Validation.Hostname))
		if err != nil {
			contextutils.LoggerFrom(ctx).Error(errors.New(fmt.Sprintf("could not create TLS config, err: %s", err)))
			return &policyIr
		}
		typedConfig, err := utils.MessageToAny(tlsCfg)
		if err != nil {
			contextutils.LoggerFrom(ctx).Error(errors.New(fmt.Sprintf("could not convert TLS config to proto, err: %s", err)))
			return &policyIr
		}

		policyIr.transportSocket = &envoy_config_core_v3.TransportSocket{
			Name: wellknown.TransportSocketTls,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
				TypedConfig: typedConfig,
			},
		}
		return &policyIr
	}
}

func convertTargetRefs(targetRefs []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName) []ir.PolicyTargetRef {
	return []ir.PolicyTargetRef{{
		Kind:  string(targetRefs[0].Kind),
		Name:  string(targetRefs[0].Name),
		Group: string(targetRefs[0].Group),
	}}
}
