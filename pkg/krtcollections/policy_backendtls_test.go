package krtcollections

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
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
	ct time.Time
}

func (p testPolicyIR) CreationTime() time.Time {
	return p.ct
}

func (p testPolicyIR) Equals(in any) bool {
	other, ok := in.(testPolicyIR)
	return ok && p.ct.Equal(other.ct)
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
