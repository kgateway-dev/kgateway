package endpointpicker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
)

func makeBasePool(ns, svcName string) *infextv1a2.InferencePool {
	return &infextv1a2.InferencePool{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns},
		Spec: infextv1a2.InferencePoolSpec{
			EndpointPickerConfig: infextv1a2.EndpointPickerConfig{
				ExtensionRef: &infextv1a2.Extension{
					ExtensionReference: infextv1a2.ExtensionReference{
						Name:       infextv1a2.ObjectName(svcName),
						Kind:       ptr.To(infextv1a2.Kind("Service")),
						Group:      ptr.To(infextv1a2.Group("")),
						PortNumber: ptr.To(infextv1a2.PortNumber(80)),
					},
				},
			},
		},
	}
}

func TestValidatePool(t *testing.T) {
	const ns = "default"
	const svcName = "test-svc"

	tests := []struct {
		name       string
		modify     func(pool *infextv1a2.InferencePool)
		includeSvc bool
		wantErrs   int
	}{
		{
			name:       "missing ExtensionRef",
			modify:     func(p *infextv1a2.InferencePool) { p.Spec.EndpointPickerConfig.ExtensionRef = nil },
			includeSvc: true, // service presence doesn’t matter here
			wantErrs:   1,
		},
		{
			name: "unsupported Group",
			modify: func(p *infextv1a2.InferencePool) {
				p.Spec.EndpointPickerConfig.ExtensionRef.ExtensionReference.Group = ptr.To(infextv1a2.Group("foo.example.com"))
			},
			includeSvc: true,
			wantErrs:   1,
		},
		{
			name: "unsupported Kind",
			modify: func(p *infextv1a2.InferencePool) {
				p.Spec.EndpointPickerConfig.ExtensionRef.ExtensionReference.Kind = ptr.To(infextv1a2.Kind("ConfigMap"))
			},
			includeSvc: true,
			wantErrs:   1,
		},
		{
			name: "port number too small",
			modify: func(p *infextv1a2.InferencePool) {
				p.Spec.EndpointPickerConfig.ExtensionRef.ExtensionReference.PortNumber = ptr.To(infextv1a2.PortNumber(0))
			},
			includeSvc: true,
			wantErrs:   1,
		},
		{
			name: "service not found",
			modify: func(p *infextv1a2.InferencePool) {
				p.Spec.EndpointPickerConfig.ExtensionRef.ExtensionReference.Name = infextv1a2.ObjectName("missing-svc")
			},
			includeSvc: false,
			wantErrs:   1,
		},
		{
			name:       "happy path",
			modify:     func(_ *infextv1a2.InferencePool) {},
			includeSvc: true,
			wantErrs:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build the pool
			pool := makeBasePool(ns, svcName)
			tc.modify(pool)

			// Collect only the Service input(s), since validatePool() only consumes the Service collection.
			var inputs []any
			if tc.includeSvc {
				inputs = []any{
					&corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      svcName,
							Namespace: ns,
						},
					},
				}
			}

			// Create the mock and grab the real krt.Collection[*corev1.Service].
			mock := krttest.NewMock(t, inputs)
			services := krttest.GetMockCollection[*corev1.Service](mock)

			// Wait until the Service collection has synced.
			services.WaitUntilSynced(context.Background().Done())

			errs := validatePool(pool, services)

			// Assert on the number of errors
			assert.Len(t, errs, tc.wantErrs, "validatePool() errors = %v", errs)
		})
	}
}
