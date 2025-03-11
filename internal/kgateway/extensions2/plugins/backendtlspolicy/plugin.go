package backendtlspolicy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/solo-io/go-utils/contextutils"
	"istio.io/istio/pkg/kube/krt"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	kgwellknown "github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

type backendTlsPolicy struct {
	ct              time.Time
	spec            gwapiv1a3.BackendTLSPolicySpec
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
	// spec has several nested slices, use DeepEqual for now
	specEq := reflect.DeepEqual(d.spec, d2.spec)
	socketEq := proto.Equal(d.transportSocket, d2.transportSocket)
	return specEq && socketEq
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	// TODO: register types directly rather than rely on dynamic client
	col := krtutil.SetupCollectionDynamic[gwapiv1a3.BackendTLSPolicy](
		ctx,
		commoncol.Client,
		gwapiv1a3.SchemeGroupVersion.WithResource("backendtlspolicies"),
		commoncol.KrtOpts.ToOptions("BackendTLSPolicy")...,
	)
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
	}, commoncol.KrtOpts.ToOptions("BackedTLSPolicyIRs")...)

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
			ct:   policyCR.CreationTimestamp.Time,
			spec: spec,
		}

		if len(spec.Validation.CACertificateRefs) == 0 {
			return &policyIr
		}

		certRef := spec.Validation.CACertificateRefs[0]
		key := fmt.Sprintf("%s/%s", policyCR.Namespace, certRef.Name)
		cfgmap := krt.FetchOne(krtctx, cfgmaps, krt.FilterKey(key))
		if cfgmap == nil {
			contextutils.LoggerFrom(ctx).Error(errors.New(fmt.Sprintf("configmap %s not found", key)))
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
