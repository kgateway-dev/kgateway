package endpointpicker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func TestValidatePool(t *testing.T) {
	const (
		ns      = "default"
		svcName = "test-svc"
	)

	tests := []struct {
		name       string
		modifyPool func(p *inf.InferencePool)
		svc        *corev1.Service
		wantErrs   int
	}{
		{
			name: "unsupported Group",
			modifyPool: func(p *inf.InferencePool) {
				p.Spec.EndpointPickerRef.Group = ptr.To(inf.Group("foo.example.com"))
			},
			svc:      makeSvc(ns, svcName, 80, corev1.ProtocolTCP, corev1.ServiceTypeClusterIP),
			wantErrs: 1,
		},
		{
			name: "unsupported Kind",
			modifyPool: func(p *inf.InferencePool) {
				p.Spec.EndpointPickerRef.Kind = inf.Kind(wellknown.ConfigMapGVK.Kind)
			},
			svc:      makeSvc(ns, svcName, 80, corev1.ProtocolTCP, corev1.ServiceTypeClusterIP),
			wantErrs: 1,
		},
		{
			name: "port unspecified",
			modifyPool: func(p *inf.InferencePool) {
				p.Spec.EndpointPickerRef.Port = nil
			},
			svc:      nil,
			wantErrs: 1,
		},
		{
			name: "service not found",
			modifyPool: func(p *inf.InferencePool) {
				p.Spec.EndpointPickerRef.Name = inf.ObjectName("missing-svc")
			},
			svc:      nil,
			wantErrs: 1,
		},
		{
			name:       "happy path",
			modifyPool: func(_ *inf.InferencePool) {},
			svc:        makeSvc(ns, svcName, 80, corev1.ProtocolTCP, corev1.ServiceTypeClusterIP),
			wantErrs:   0,
		},
		{
			name:       "ExternalName service rejected",
			modifyPool: func(_ *inf.InferencePool) {},
			svc:        makeSvc(ns, svcName, 80, corev1.ProtocolTCP, corev1.ServiceTypeExternalName),
			wantErrs:   1,
		},
		{
			name:       "UDP port not accepted",
			modifyPool: func(_ *inf.InferencePool) {},
			svc:        makeSvc(ns, svcName, 80, corev1.ProtocolUDP, corev1.ServiceTypeClusterIP),
			wantErrs:   1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build the pool
			pool := makeBasePool(ns, svcName)
			tc.modifyPool(pool)

			// Collect only the Service input(s), since validatePool() only consumes the Service collection.
			var inputs []any
			if tc.svc != nil {
				inputs = append(inputs, tc.svc)
			}
			// Create a dummy LocalityPod
			corePod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      "fake-pod",
					Labels:    map[string]string{"foo": "bar"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.1.1.1",
				},
			}
			fakeLP := krtcollections.LocalityPod{
				Named:           krt.NewNamed(corePod),
				AugmentedLabels: corePod.Labels,
				Addresses:       []string{corePod.Status.PodIP},
			}
			inputs = append(inputs, fakeLP)

			// Create the mock and grab the Pod and Service collections.
			mock := krttest.NewMock(t, inputs)
			svcCol := krttest.GetMockCollection[*corev1.Service](mock)
			podCol := krttest.GetMockCollection[krtcollections.LocalityPod](mock)

			// Wait until the Pod and Service collections have synced.
			svcCol.WaitUntilSynced(context.Background().Done())
			podCol.WaitUntilSynced(context.Background().Done())

			// Assert on the number of errors
			errs := validatePool(pool, svcCol)
			assert.Lenf(t, errs, tc.wantErrs,
				"validatePool() returned %d errors: %v", len(errs), errs)
		})
	}
}

func makeBasePool(ns, svcName string) *inf.InferencePool {
	return &inf.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: ns,
		},
		Spec: inf.InferencePoolSpec{
			Selector: inf.LabelSelector{
				MatchLabels: map[inf.LabelKey]inf.LabelValue{"foo": "bar"},
			},
			TargetPorts: []inf.Port{{Number: 9002}},
			EndpointPickerRef: inf.EndpointPickerRef{
				Group: ptr.To(inf.Group("")),
				Kind:  inf.Kind(wellknown.ServiceKind),
				Name:  inf.ObjectName(svcName),
				Port:  &inf.Port{Number: inf.PortNumber(80)},
			},
		},
	}
}

func makeSvc(ns, name string, port int32, proto corev1.Protocol, typ corev1.ServiceType) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Type: typ,
			Ports: []corev1.ServicePort{{
				Name:     "test-port",
				Port:     port,
				Protocol: proto,
			}},
		},
	}
}
