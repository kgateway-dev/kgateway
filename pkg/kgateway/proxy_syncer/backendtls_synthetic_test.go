package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	pluginutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
)

type backendTLSTestPolicyIR struct {
	ct           time.Time
	transportSNI string
}

func (p backendTLSTestPolicyIR) CreationTime() time.Time {
	return p.ct
}

func (p backendTLSTestPolicyIR) Equals(in any) bool {
	other, ok := in.(backendTLSTestPolicyIR)
	return ok && p.ct.Equal(other.ct) && p.transportSNI == other.transportSNI
}

func TestPerClientClustersUpdateWhenBackendTLSPolicyAddedLater(t *testing.T) {
	ctx := t.Context()

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "https", Port: 443, TargetPort: intstr.FromInt32(8443)},
			},
		},
	}

	fakeClient := apifake.NewClient(t, service)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	serviceClient := kclient.NewFiltered[*corev1.Service](
		fakeClient,
		kclient.Filter{ObjectFilter: fakeClient.ObjectFilter()},
	)
	services := krt.WrapClient(serviceClient, krtopts.ToOptions("Services")...)

	backendTLSPolicyClient := kclient.NewFilteredDelayed[*gwv1.BackendTLSPolicy](
		fakeClient,
		wellknown.BackendTLSPolicyGVR,
		kclient.Filter{ObjectFilter: fakeClient.ObjectFilter()},
	)
	backendTLSPolicies := krt.WrapClient(backendTLSPolicyClient, krtopts.ToOptions("BackendTLSPolicy")...)
	policyCol := krt.NewCollection(backendTLSPolicies, func(kctx krt.HandlerContext, policy *gwv1.BackendTLSPolicy) *ir.PolicyWrapper {
		return &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     wellknown.BackendTLSPolicyGVK.Group,
				Kind:      wellknown.BackendTLSPolicyGVK.Kind,
				Namespace: policy.Namespace,
				Name:      policy.Name,
			},
			Policy: policy,
			PolicyIR: backendTLSTestPolicyIR{
				ct:           policy.CreationTimestamp.Time,
				transportSNI: string(policy.Spec.Validation.Hostname),
			},
			TargetRefs: pluginutils.TargetRefsToPolicyRefsWithSectionNameV1(policy.Spec.TargetRefs),
		}
	}, krtopts.ToOptions("BackendTLSPolicyWrappers")...)

	policies := krtcollections.NewPolicyIndex(
		krtopts,
		sdk.ContributesPolicies{
			wellknown.BackendTLSPolicyGVK.GroupKind(): {
				Name:     "BackendTLSPolicy",
				Policies: policyCol,
				ProcessBackend: func(ctx context.Context, pol ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
					tlsPolicy := pol.(backendTLSTestPolicyIR)
					out.TransportSocket = &envoycorev3.TransportSocket{Name: tlsPolicy.transportSNI}
				},
				MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
					return pols[0]
				},
			},
		},
		apisettings.Settings{},
	)
	refgrants := krtcollections.NewRefGrantIndex(krt.NewStaticCollection[*gwv1b1.ReferenceGrant](nil, nil, krtopts.ToOptions("RefGrants")...))
	backends := krtcollections.NewBackendIndex(krtopts, policies, refgrants)
	serviceBackends := krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.BackendObjectIR {
		out := make([]ir.BackendObjectIR, 0, len(svc.Spec.Ports))
		for _, port := range svc.Spec.Ports {
			backend := ir.NewBackendObjectIR(ir.ObjectSource{
				Group:     "",
				Kind:      "Service",
				Namespace: svc.Namespace,
				Name:      svc.Name,
			}, port.Port, "")
			backend.Obj = svc
			backend.PortName = port.Name
			out = append(out, backend)
		}
		return out
	}, krtopts.ToOptions("ServiceBackends")...)
	backends.AddBackends(schema.GroupKind{Group: "", Kind: "Service"}, serviceBackends)

	ucc := ir.NewUniqlyConnectedClient("test-role", "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniqlyConnectedClient{ucc}, krtopts.ToOptions("UniqueClients")...)
	finalBackends := krt.JoinCollection(backends.BackendsWithPolicy(), krtopts.ToOptions("FinalBackends")...)
	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "", Kind: "Service"}: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
		ContributedPolicies: policiesIndexPlugins(policyCol),
	}
	clusters := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)

	fakeClient.RunAndWait(ctx.Done())

	require.Eventually(t, func() bool {
		return services.HasSynced() && backendTLSPolicies.HasSynced() && policyCol.HasSynced() && backends.HasSynced()
	}, 5*time.Second, 50*time.Millisecond)

	require.Eventually(t, func() bool {
		fetched := clusters.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
		return len(fetched) == 1 && fetched[0].Cluster != nil && fetched[0].Cluster.GetTransportSocket() == nil
	}, 5*time.Second, 50*time.Millisecond)

	older := time.Now()
	policy1 := &gwv1.BackendTLSPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1.GroupVersion.String(),
			Kind:       wellknown.BackendTLSPolicyGVK.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "backend-tls-older",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(older),
			Generation:        1,
		},
		Spec: gwv1.BackendTLSPolicySpec{
			TargetRefs: []gwv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gwv1.LocalPolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "backend-service",
				},
			}},
			Validation: gwv1.BackendTLSPolicyValidation{
				Hostname: "other.example.com",
			},
		},
	}
	_, err := fakeClient.GatewayAPI().GatewayV1().BackendTLSPolicies("default").Create(ctx, policy1, metav1.CreateOptions{})
	require.NoError(t, err)

	policy2 := &gwv1.BackendTLSPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1.GroupVersion.String(),
			Kind:       wellknown.BackendTLSPolicyGVK.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "backend-tls-newer",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(older.Add(time.Second)),
			Generation:        1,
		},
		Spec: gwv1.BackendTLSPolicySpec{
			TargetRefs: []gwv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gwv1.LocalPolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "backend-service",
				},
			}},
			Validation: gwv1.BackendTLSPolicyValidation{
				Hostname: "abc.example.com",
			},
		},
	}
	_, err = fakeClient.GatewayAPI().GatewayV1().BackendTLSPolicies("default").Create(ctx, policy2, metav1.CreateOptions{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		fetched := clusters.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
		if len(fetched) != 1 || fetched[0].Cluster == nil {
			return false
		}
		return fetched[0].Cluster.GetTransportSocket() != nil && fetched[0].Cluster.GetTransportSocket().GetName() == "other.example.com"
	}, 5*time.Second, 50*time.Millisecond)
}

func policiesIndexPlugins(policyCol krt.Collection[ir.PolicyWrapper]) map[schema.GroupKind]sdk.PolicyPlugin {
	return map[schema.GroupKind]sdk.PolicyPlugin{
		wellknown.BackendTLSPolicyGVK.GroupKind(): {
			Name: "BackendTLSPolicy",
			ProcessBackend: func(ctx context.Context, pol ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
				tlsPolicy := pol.(backendTLSTestPolicyIR)
				out.TransportSocket = &envoycorev3.TransportSocket{Name: tlsPolicy.transportSNI}
			},
			MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
				return pols[0]
			},
			Policies: policyCol,
		},
	}
}
