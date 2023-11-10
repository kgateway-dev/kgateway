package reports_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/reports"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Reports", func() {

	BeforeEach(func() {
	})

	Describe("Build Gateway Status", func() {
		It("should build all positive condtions with an empty report", func() {
			gw := gw()
			rm := reports.NewReportMap()
			status := rm.BuildGWStatus(context.Background(), *gw)

			Expect(status).NotTo(BeNil())
			Expect(status.Conditions).To(HaveLen(2))
			Expect(status.Listeners).To(HaveLen(1))
			// Expect(err).NotTo(HaveOccurred())
			// Expect(backend).NotTo(BeNil())
			// Expect(backend.GetName()).To(Equal("foo"))
			// Expect(backend.GetNamespace()).To(Equal("default"))
		})

	})
})

func httpRoute() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}

}

func gw() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				gwv1.Listener{
					Name: "http",
				},
			},
		},
	}

}

func secret(ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "foo",
		},
	}
}

func svc(ns string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "foo",
		},
	}
}

func nsptr(s string) *gwv1.Namespace {
	var ns gwv1.Namespace = gwv1.Namespace(s)
	return &ns
}
