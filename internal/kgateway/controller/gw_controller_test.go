package controller_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	api "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/test/gomega/assertions"
)

var _ = Describe("GwController", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		ctx              context.Context
		cancel           context.CancelFunc
		goroutineMonitor *assertions.GoRoutineMonitor
	)

	BeforeEach(func() {
		goroutineMonitor = assertions.NewGoRoutineMonitor()
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		cancel, err = createManager(ctx, inferenceExt, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel()
		waitForGoroutinesToFinish(goroutineMonitor)
	})

	DescribeTable(
		"should add status to gateway",
		func(gwClass string) {
			same := api.NamespacesFromSame
			gwName := "gw-" + gwClass
			gw := api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwName,
					Namespace: "default",
				},
				Spec: api.GatewaySpec{
					Addresses: []api.GatewaySpecAddress{{
						Type:  ptr.To(api.IPAddressType),
						Value: "127.0.0.1",
					}},
					GatewayClassName: api.ObjectName(gwClass),
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

			if gwClass != selfManagedGatewayClassName {
				svc := waitForGatewayService(ctx, &gw)

				// Need to update the status of the service
				svc.Status.LoadBalancer = corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{{
						IP: "127.0.0.1",
					}},
				}
				Eventually(func() error {
					return k8sClient.Status().Update(ctx, &svc)
				}, timeout, interval).Should(Succeed())
			}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: gwName, Namespace: "default"}, &gw)
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
		},
		Entry("default gateway class", gatewayClassName),
		Entry("alternative gateway class", altGatewayClassName),
		Entry("self managed gateway", selfManagedGatewayClassName),
	)
})
