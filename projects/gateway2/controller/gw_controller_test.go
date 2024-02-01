package controller_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	api "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("GwController", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	It("should add status to gateway", func() {
		same := api.NamespacesFromSame
		gw := api.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gw",
				Namespace: "default",
			},
			Spec: api.GatewaySpec{
				GatewayClassName: api.ObjectName(gatewayClassName),
				Listeners: []api.Listener{{
					Protocol: "HTTP",
					Port:     80,
					AllowedRoutes: &api.AllowedRoutes{
						Namespaces: &api.RouteNamespaces{
							From: &same,
						},
					},
					Name: "listener",
				}},
			},
		}
		err := k8sClient.Create(ctx, &gw)
		Expect(err).NotTo(HaveOccurred())

		// Wait for service to be created
		var svc corev1.Service
		Eventually(func() bool {
			var createdServices corev1.ServiceList
			err := k8sClient.List(ctx, &createdServices)
			if err != nil {
				return false
			}
			for _, svc = range createdServices.Items {
				if len(svc.ObjectMeta.OwnerReferences) == 1 && svc.ObjectMeta.OwnerReferences[0].UID == gw.UID {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "service not created")
		Expect(svc.Spec.ClusterIP).NotTo(BeEmpty())

		// Need to update the status of the service
		svc.Status.LoadBalancer = corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{{
				IP: "127.0.0.1",
			}},
		}
		Expect(k8sClient.Status().Update(ctx, &svc)).NotTo(HaveOccurred())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "gw", Namespace: "default"}, &gw)
			if err != nil {
				return false
			}
			if len(gw.Status.Addresses) == 0 {
				return false
			}
			return true
		}, timeout, interval).Should(BeTrue())

		Expect(gw.Status.Addresses).To(HaveLen(1))
		Expect(*gw.Status.Addresses[0].Type).To(Equal(api.IPAddressType))
		Expect(gw.Status.Addresses[0].Value).To(Equal("127.0.0.1"))

	})

})
