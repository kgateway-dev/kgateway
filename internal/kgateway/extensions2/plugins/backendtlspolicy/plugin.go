package backendtlspolicy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/avast/retry-go"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/solo-io/go-utils/contextutils"
	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/slices"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	plug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/proxy_syncer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	kgwellknown "github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

var (
	backendTlsPolicyGvr       = gwv1a3.SchemeGroupVersion.WithResource("backendtlspolicies")
	backendTlsPolicyGroupKind = kgwellknown.BackendTLSPolicyGVK
)

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
	kubeclient.Register[*gwv1a3.BackendTLSPolicy](
		backendTlsPolicyGvr,
		backendTlsPolicyGroupKind,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1alpha3().BackendTLSPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1alpha3().BackendTLSPolicies(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) plug.Plugin {
	registerTypes()
	inf := kclient.NewDelayedInformer[*gwv1a3.BackendTLSPolicy](commoncol.Client, backendTlsPolicyGvr, kubetypes.StandardInformer, kclient.Filter{})
	col := krt.WrapClient(inf, commoncol.KrtOpts.ToOptions("BackendTLSPolicy")...)

	translate := buildTranslateFunc(ctx, commoncol.ConfigMaps)
	tlsPolicyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *gwv1a3.BackendTLSPolicy) *ir.PolicyWrapper {
		tlsPolicyIR, err := translate(krtctx, i)
		var pol = &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     backendTlsPolicyGroupKind.Group,
				Kind:      backendTlsPolicyGroupKind.Kind,
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Policy:     i,
			PolicyIR:   tlsPolicyIR,
			TargetRefs: convertTargetRefs(i.Spec.TargetRefs),
		}
		if err != nil {
			pol.Errors = []error{err}
		}
		return pol
	}, commoncol.KrtOpts.ToOptions("BackendTLSPolicyIRs")...)

	return plug.Plugin{
		ContributesPolicies: map[schema.GroupKind]plug.PolicyPlugin{
			backendTlsPolicyGroupKind.GroupKind(): {
				Name:                "BackendTLSPolicy",
				Policies:            tlsPolicyCol,
				ProcessBackend:      ProcessBackend,
				ProcessPolicyStatus: buildProcessStatus(commoncol.CrudClient),
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
) func(krtctx krt.HandlerContext, i *gwv1a3.BackendTLSPolicy) (*backendTlsPolicy, error) {
	return func(krtctx krt.HandlerContext, policyCR *gwv1a3.BackendTLSPolicy) (*backendTlsPolicy, error) {
		spec := policyCR.Spec
		policyIr := backendTlsPolicy{
			ct: policyCR.CreationTimestamp.Time,
		}

		if len(spec.Validation.CACertificateRefs) == 0 {
			return &policyIr, nil
		}

		certRef := spec.Validation.CACertificateRefs[0]
		nn := types.NamespacedName{
			Name:      string(certRef.Name),
			Namespace: policyCR.Namespace,
		}
		cfgmap := krt.FetchOne(krtctx, cfgmaps, krt.FilterObjectName(nn))
		if cfgmap == nil {
			polErr := errors.New(fmt.Sprintf("configmap %s not found", nn))
			contextutils.LoggerFrom(ctx).Error(polErr)
			return &policyIr, polErr
		}

		tlsCfg, err := ResolveUpstreamSslConfig(*cfgmap, string(spec.Validation.Hostname))
		if err != nil {
			polErr := errors.New(fmt.Sprintf("could not create TLS config, err: %s", err))
			contextutils.LoggerFrom(ctx).Error(polErr)
			return &policyIr, polErr
		}
		typedConfig, err := utils.MessageToAny(tlsCfg)
		if err != nil {
			polErr := errors.New(fmt.Sprintf("could not convert TLS config to proto, err: %s", err))
			contextutils.LoggerFrom(ctx).Error(polErr)
			return &policyIr, polErr
		}

		policyIr.transportSocket = &envoy_config_core_v3.TransportSocket{
			Name: wellknown.TransportSocketTls,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
				TypedConfig: typedConfig,
			},
		}
		return &policyIr, nil
	}
}

func buildProcessStatus(cl client.Client) func(ctx context.Context, gkStr string, polReport plug.PolicyReport) {
	return func(ctx context.Context, gkStr string, polReport plug.PolicyReport) {
		if gkStr != backendTlsPolicyGroupKind.GroupKind().String() {
			return
		}
		ctx = contextutils.WithLogger(ctx, "backendTlsPolicyStatus")
		logger := contextutils.LoggerFrom(ctx)
		for ref, rpt := range polReport {
			// get existing policy
			res := gwv1a3.BackendTLSPolicy{}
			resNN := types.NamespacedName{
				Name:      ref.Name,
				Namespace: ref.Namespace,
			}
			err := cl.Get(ctx, resNN, &res)
			if err != nil {
				logger.Error("error getting backendtlspolicy", err.Error())
				continue
			}

			ancestors := make([]gwv1a2.PolicyAncestorStatus, 0, len(rpt))
			for objSrc, policyErrs := range rpt {
				newAncestor := gwv1.ParentReference{
					Group: (*gwv1.Group)(&objSrc.Group),
					Kind:  (*gwv1.Kind)(&objSrc.Kind),
					Name:  gwv1.ObjectName(objSrc.Name),
				}
				pas := gwv1a2.PolicyAncestorStatus{
					AncestorRef:    newAncestor,
					ControllerName: kgwellknown.GatewayControllerName,
				}

				// check if existing status has this ancestor
				conditions := make([]metav1.Condition, 0, 1)
				foundAncestor := slices.FindFunc(res.Status.Ancestors, func(in gwv1a2.PolicyAncestorStatus) bool {
					groupEq := ptrEquals(newAncestor.Group, in.AncestorRef.Group)
					kindEq := ptrEquals(newAncestor.Kind, in.AncestorRef.Kind)
					nameEq := newAncestor.Name == in.AncestorRef.Name
					return groupEq && kindEq && nameEq
				})
				if foundAncestor != nil {
					copy(conditions, foundAncestor.Conditions)
				}
				meta.SetStatusCondition(&conditions, proxy_syncer.BuildPolicyCondition(policyErrs))
				pas.Conditions = conditions

				ancestors = append(ancestors, pas)
			}

			newStatus := gwv1a2.PolicyStatus{
				Ancestors: ancestors,
			}
			// if the status is up-to-date, nothing to do
			if reflect.DeepEqual(newStatus, res.Status) {
				continue
			}

			res.Status = newStatus
			err = retry.Do(
				func() error {
					if err := cl.Status().Patch(ctx, &res, client.Merge); err != nil {
						logger.Error(err)
						return err
					}
					return nil
				},
				retry.Attempts(5),
				retry.Delay(100*time.Millisecond),
				retry.DelayType(retry.BackOffDelay),
			)
			if err != nil {
				logger.Errorw(
					"all attempts failed updating backendtlspolicy status",
					"BackendTLSPolicy",
					resNN.String(),
					"error",
					err,
				)
			}
		}
	}
}

func convertTargetRefs(targetRefs []gwv1a2.LocalPolicyTargetReferenceWithSectionName) []ir.PolicyRef {
	return []ir.PolicyRef{{
		Kind:  string(targetRefs[0].Kind),
		Name:  string(targetRefs[0].Name),
		Group: string(targetRefs[0].Group),
	}}
}

func ptrEquals[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
