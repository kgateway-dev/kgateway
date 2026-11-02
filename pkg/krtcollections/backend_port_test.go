package krtcollections

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// serviceEntryLikeBackends mimics the ServiceEntry plugin: one backend per port,
// keyed with the hostname as extraKey so it only resolves through the alias index.
func serviceEntryLikeBackends(services krt.Collection[*corev1.Service]) krt.Collection[ir.BackendObjectIR] {
	return krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.BackendObjectIR {
		objSrc := ir.ObjectSource{
			Group:     wellknown.ServiceEntryGVK.Group,
			Kind:      wellknown.ServiceEntryGVK.Kind,
			Namespace: svc.Namespace,
			Name:      svc.Name,
		}
		hostname := svc.Name + ".example.com"
		out := make([]ir.BackendObjectIR, 0, len(svc.Spec.Ports))
		for _, port := range svc.Spec.Ports {
			backend := ir.NewBackendObjectIR(objSrc, port.Port, hostname, "")
			backend.Obj = svc
			backend.Aliases = []ir.ObjectSource{
				objSrc,
				{
					Group:     wellknown.HostnameGVK.Group,
					Kind:      wellknown.HostnameGVK.Kind,
					Namespace: "",
					Name:      hostname,
				},
			}
			out = append(out, backend)
		}
		return out
	})
}

// newPortTestBackendIndex serves default/foo:8080 as both a Service and a
// ServiceEntry-like backend.
func newPortTestBackendIndex(t *testing.T) *BackendIndex {
	t.Helper()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 8080}},
		},
	}

	mock := krttest.NewMock(t, []any{svc})
	services := krttest.GetMockCollection[*corev1.Service](mock)
	policyCol := krttest.GetMockCollection[ir.PolicyWrapper](mock)
	policies := NewPolicyIndex(krtutil.KrtOptions{}, sdk.ContributesPolicies{}, apisettings.Settings{})
	refgrants := NewRefGrantIndex(krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock), apisettings.ReferenceGrantPermissive)

	backends := NewBackendIndex(krtutil.KrtOptions{}, policies, refgrants)
	backends.AddBackends(svcGk, k8sSvcUpstreams(services))
	backends.AddBackends(
		wellknown.ServiceEntryGVK.GroupKind(),
		serviceEntryLikeBackends(services),
		wellknown.HostnameGVK.GroupKind(),
		wellknown.ServiceEntryGVK.GroupKind(),
	)

	services.WaitUntilSynced(nil)
	policyCol.WaitUntilSynced(nil)
	for !backends.HasSynced() {
		time.Sleep(time.Second / 10)
	}
	return backends
}

func TestGetBackendFromRefPortErrors(t *testing.T) {
	backends := newPortTestBackendIndex(t)
	src := ir.ObjectSource{
		Group:     gwv1.GroupVersion.Group,
		Kind:      "HTTPRoute",
		Namespace: "default",
		Name:      "route",
	}

	group := func(g string) *gwv1.Group { gg := gwv1.Group(g); return &gg }
	kind := func(k string) *gwv1.Kind { kk := gwv1.Kind(k); return &kk }
	port := func(p int32) *gwv1.PortNumber { pp := gwv1.PortNumber(p); return &pp }

	cases := []struct {
		name    string
		ref     gwv1.BackendObjectReference
		wantErr string
	}{
		{
			// direct krt-key lookup on a core Service
			name:    "service wrong port",
			ref:     gwv1.BackendObjectReference{Name: "foo", Port: port(9090)},
			wantErr: "Service default/foo found, but port 9090 not defined",
		},
		{
			name:    "service missing",
			ref:     gwv1.BackendObjectReference{Name: "nope", Port: port(9090)},
			wantErr: "Service default/nope not found",
		},
		{
			// no port in the ref: must not report "port 0 not defined"
			name:    "service no port",
			ref:     gwv1.BackendObjectReference{Name: "nope"},
			wantErr: "Service default/nope not found",
		},
		{
			// resolved through the alias index, so the error must name ServiceEntry
			name: "service entry wrong port",
			ref: gwv1.BackendObjectReference{
				Group: group(wellknown.ServiceEntryGVK.Group),
				Kind:  kind(wellknown.ServiceEntryGVK.Kind),
				Name:  "foo",
				Port:  port(9090),
			},
			wantErr: "ServiceEntry default/foo found, but port 9090 not defined",
		},
		{
			name: "service entry missing",
			ref: gwv1.BackendObjectReference{
				Group: group(wellknown.ServiceEntryGVK.Group),
				Kind:  kind(wellknown.ServiceEntryGVK.Kind),
				Name:  "nope",
				Port:  port(9090),
			},
			wantErr: "ServiceEntry default/nope not found",
		},
		{
			// aliased kind with no collection of its own
			name: "hostname wrong port",
			ref: gwv1.BackendObjectReference{
				Group: group(wellknown.HostnameGVK.Group),
				Kind:  kind(wellknown.HostnameGVK.Kind),
				Name:  "foo.example.com",
				Port:  port(9090),
			},
			wantErr: "Hostname /foo.example.com found, but port 9090 not defined",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			be, err := backends.GetBackendFromRef(krt.TestingDummyContext{}, src, tc.ref)
			require.Nil(t, be, "no backend should resolve")
			require.EqualError(t, err, tc.wantErr)
		})
	}
}

func TestGetBackendFromRefResolves(t *testing.T) {
	backends := newPortTestBackendIndex(t)
	src := ir.ObjectSource{
		Group:     gwv1.GroupVersion.Group,
		Kind:      "HTTPRoute",
		Namespace: "default",
		Name:      "route",
	}
	port := gwv1.PortNumber(8080)

	be, err := backends.GetBackendFromRef(krt.TestingDummyContext{}, src, gwv1.BackendObjectReference{
		Name: "foo",
		Port: &port,
	})
	require.NoError(t, err)
	require.Equal(t, "foo", be.GetName())
}
