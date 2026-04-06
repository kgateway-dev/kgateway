package krtcollections

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	pluginutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
)

func TestPreferPortSpecificBackendTLSPolicies(t *testing.T) {
	otherGK := schema.GroupKind{Group: "test.io", Kind: "ConnectionPolicy"}
	serviceWidePolicies := []ir.PolicyAtt{
		{GroupKind: wellknown.BackendTLSPolicyGVK.GroupKind()},
		{GroupKind: otherGK},
	}
	portPolicies := []ir.PolicyAtt{
		{GroupKind: wellknown.BackendTLSPolicyGVK.GroupKind()},
	}

	filtered := preferPortSpecificBackendTLSPolicies(serviceWidePolicies, portPolicies)

	require.Len(t, filtered, 1)
	require.Equal(t, otherGK, filtered[0].GroupKind)
}

type testPolicyIR struct {
	ct           time.Time
	transportSNI string
}

func (p testPolicyIR) CreationTime() time.Time {
	return p.ct
}

func (p testPolicyIR) Equals(in any) bool {
	other, ok := in.(testPolicyIR)
	return ok && p.ct.Equal(other.ct) && p.transportSNI == other.transportSNI
}

func TestGetBackendFromRefReturnsPolicyAttachedBackend(t *testing.T) {
	now := time.Now()
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "https-1", Port: 443},
				{Name: "https-2", Port: 8443},
			},
		},
	}
	serviceWide := ir.PolicyWrapper{
		ObjectSource: ir.ObjectSource{
			Group:     wellknown.BackendTLSPolicyGVK.Group,
			Kind:      wellknown.BackendTLSPolicyGVK.Kind,
			Namespace: "default",
			Name:      "service-wide",
		},
		Policy: &gwv1.BackendTLSPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "service-wide",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(now),
				Generation:        1,
			},
		},
		PolicyIR: testPolicyIR{ct: now},
		TargetRefs: []ir.PolicyRef{{
			Group: "",
			Kind:  "Service",
			Name:  "backend-service",
		}},
	}
	portSpecific := ir.PolicyWrapper{
		ObjectSource: ir.ObjectSource{
			Group:     wellknown.BackendTLSPolicyGVK.Group,
			Kind:      wellknown.BackendTLSPolicyGVK.Kind,
			Namespace: "default",
			Name:      "port-specific",
		},
		Policy: &gwv1.BackendTLSPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "port-specific",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(now.Add(time.Second)),
				Generation:        1,
			},
		},
		PolicyIR: testPolicyIR{ct: now.Add(time.Second)},
		TargetRefs: []ir.PolicyRef{{
			Group:       "",
			Kind:        "Service",
			Name:        "backend-service",
			SectionName: "https-1",
		}},
	}

	mock := krttest.NewMock(t, []any{service, serviceWide, portSpecific})
	services := krttest.GetMockCollection[*corev1.Service](mock)
	policyCol := krttest.GetMockCollection[ir.PolicyWrapper](mock)
	policies := NewPolicyIndex(
		krtutil.KrtOptions{},
		sdk.ContributesPolicies{
			wellknown.BackendTLSPolicyGVK.GroupKind(): {
				Policies: policyCol,
				ProcessBackend: func(ctx context.Context, pol ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
				},
			},
		},
		apisettings.Settings{},
	)
	refgrants := NewRefGrantIndex(krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock))
	backends := NewBackendIndex(krtutil.KrtOptions{}, policies, refgrants)
	serviceBackends := krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.BackendObjectIR {
		out := make([]ir.BackendObjectIR, 0, len(svc.Spec.Ports))
		for _, port := range svc.Spec.Ports {
			backend := ir.NewBackendObjectIR(ir.ObjectSource{
				Group:     svcGk.Group,
				Kind:      svcGk.Kind,
				Namespace: svc.Namespace,
				Name:      svc.Name,
			}, port.Port, "")
			backend.Obj = svc
			backend.PortName = port.Name
			out = append(out, backend)
		}
		return out
	})
	backends.AddBackends(svcGk, serviceBackends)

	services.WaitUntilSynced(nil)
	policyCol.WaitUntilSynced(nil)
	for !backends.HasSynced() || !policies.HasSynced() || !refgrants.HasSynced() {
		time.Sleep(time.Second / 10)
	}

	src := ir.ObjectSource{
		Group:     gwv1.GroupVersion.Group,
		Kind:      "HTTPRoute",
		Namespace: "default",
		Name:      "route",
	}

	backend443, err := backends.GetBackendFromRef(krt.TestingDummyContext{}, src, gwv1.BackendObjectReference{
		Name: "backend-service",
		Port: ptr.To(gwv1.PortNumber(443)),
	})
	require.NoError(t, err)
	require.Len(t, backend443.AttachedPolicies.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()], 1)
	require.Equal(t, "port-specific", backend443.AttachedPolicies.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()][0].PolicyRef.Name)
	require.Equal(t, "https-1", backend443.AttachedPolicies.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()][0].PolicyRef.SectionName)

	backend8443, err := backends.GetBackendFromRef(krt.TestingDummyContext{}, src, gwv1.BackendObjectReference{
		Name: "backend-service",
		Port: ptr.To(gwv1.PortNumber(8443)),
	})
	require.NoError(t, err)
	require.Len(t, backend8443.AttachedPolicies.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()], 1)
	require.Equal(t, "service-wide", backend8443.AttachedPolicies.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()][0].PolicyRef.Name)
	require.Empty(t, backend8443.AttachedPolicies.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()][0].PolicyRef.SectionName)
}

func TestBackendPoliciesUpdateWhenBackendTLSPolicyCreatedAfterService(t *testing.T) {
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
			Selector: map[string]string{"app": "backend"},
			Ports: []corev1.ServicePort{
				{Name: "https", Port: 443, TargetPort: intstrFromInt(8443)},
			},
		},
	}

	fakeClient := apifake.NewClient(t, service)

	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	settings := apisettings.Settings{}

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
			PolicyIR: testPolicyIR{
				ct:           policy.CreationTimestamp.Time,
				transportSNI: string(policy.Spec.Validation.Hostname),
			},
			TargetRefs: pluginutils.TargetRefsToPolicyRefsWithSectionNameV1(policy.Spec.TargetRefs),
		}
	}, krtopts.ToOptions("BackendTLSPolicyWrappers")...)

	policies := NewPolicyIndex(
		krtopts,
		sdk.ContributesPolicies{
			wellknown.BackendTLSPolicyGVK.GroupKind(): {
				Name:     "BackendTLSPolicy",
				Policies: policyCol,
				ProcessBackend: func(ctx context.Context, pol ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
					tlsPolicy := pol.(testPolicyIR)
					out.TransportSocket = &envoycorev3.TransportSocket{Name: tlsPolicy.transportSNI}
				},
				MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
					return pols[0]
				},
			},
		},
		settings,
	)
	refgrants := NewRefGrantIndex(krt.NewStaticCollection[*gwv1b1.ReferenceGrant](nil, nil, krtopts.ToOptions("RefGrants")...))
	backends := NewBackendIndex(krtopts, policies, refgrants)
	serviceBackends := krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.BackendObjectIR {
		out := make([]ir.BackendObjectIR, 0, len(svc.Spec.Ports))
		for _, port := range svc.Spec.Ports {
			backend := ir.NewBackendObjectIR(ir.ObjectSource{
				Group:     svcGk.Group,
				Kind:      svcGk.Kind,
				Namespace: svc.Namespace,
				Name:      svc.Name,
			}, port.Port, "")
			backend.Obj = svc
			backend.PortName = port.Name
			out = append(out, backend)
		}
		return out
	}, krtopts.ToOptions("ServiceBackends")...)
	backends.AddBackends(svcGk, serviceBackends)
	fakeClient.RunAndWait(ctx.Done())

	require.Eventually(t, func() bool {
		return services.HasSynced() && backendTLSPolicies.HasSynced() && policyCol.HasSynced() && backends.HasSynced() && policies.HasSynced()
	}, 5*time.Second, 50*time.Millisecond)

	src := ir.ObjectSource{
		Group:     gwv1.GroupVersion.Group,
		Kind:      "HTTPRoute",
		Namespace: "default",
		Name:      "route",
	}

	backend, err := backends.GetBackendFromRef(krt.TestingDummyContext{}, src, gwv1.BackendObjectReference{
		Name: "backend-service",
		Port: ptr.To(gwv1.PortNumber(443)),
	})
	require.NoError(t, err)
	require.Empty(t, backend.AttachedPolicies.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()])

	createdAt := metav1.NewTime(time.Now())
	backendTLSPolicy := &gwv1.BackendTLSPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1.GroupVersion.String(),
			Kind:       wellknown.BackendTLSPolicyGVK.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "backend-tls",
			Namespace:         "default",
			CreationTimestamp: createdAt,
			Generation:        1,
		},
		Spec: gwv1.BackendTLSPolicySpec{
			TargetRefs: []gwv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gwv1.LocalPolicyTargetReference{
						Group: "",
						Kind:  "Service",
						Name:  "backend-service",
					},
				},
			},
			Validation: gwv1.BackendTLSPolicyValidation{
				Hostname: "abc.example.com",
			},
		},
	}
	_, err = fakeClient.GatewayAPI().GatewayV1().BackendTLSPolicies("default").Create(ctx, backendTLSPolicy, metav1.CreateOptions{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		policy := krt.FetchOne(krt.TestingDummyContext{}, backendTLSPolicies, krt.FilterKey("default/backend-tls"))
		return policy != nil
	}, 5*time.Second, 50*time.Millisecond)

	require.Eventually(t, func() bool {
		updatedBackend, err := backends.GetBackendFromRef(krt.TestingDummyContext{}, src, gwv1.BackendObjectReference{
			Name: "backend-service",
			Port: ptr.To(gwv1.PortNumber(443)),
		})
		if err != nil {
			return false
		}
		attachments := updatedBackend.AttachedPolicies.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()]
		return len(attachments) == 1 && attachments[0].PolicyRef.Name == "backend-tls"
	}, 5*time.Second, 50*time.Millisecond)
}

func intstrFromInt(v int32) intstr.IntOrString {
	return intstr.FromInt32(v)
}
