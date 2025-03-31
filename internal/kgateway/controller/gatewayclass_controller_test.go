package controller_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("GatewayClass Status Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		var err error
		cancel, err = createManager(ctx, nil, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		// ensure goroutines cleanup
		Eventually(func() bool { return true }).WithTimeout(3 * time.Second).Should(BeTrue())
	})

	Context("GatewayClass reconciliation", func() {
		var (
			gc *apiv1.GatewayClass
		)
		BeforeEach(func() {
			gc = &apiv1.GatewayClass{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gc); err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue(), "GatewayClass %s not found", gatewayClassName)
		})

		It("should set the Accepted=True condition type", func() {
			Expect(gc.Status.Conditions).Should(HaveLen(2))
			Expect(gc.Status.Conditions[0].Type).Should(Equal(string(apiv1.GatewayClassConditionStatusAccepted)))
			Expect(gc.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
			Expect(gc.Status.Conditions[0].Reason).Should(Equal(string(apiv1.GatewayClassReasonAccepted)))
			Expect(gc.Status.Conditions[0].Message).Should(ContainSubstring(`accepted by kgateway controller`))
		})

		It("should set the SupportedVersion=True condition type", func() {
			Expect(gc.Status.Conditions).Should(HaveLen(2))
			Expect(gc.Status.Conditions[1].Type).Should(Equal(string(apiv1.GatewayClassConditionStatusSupportedVersion)))
			Expect(gc.Status.Conditions[1].Status).Should(Equal(metav1.ConditionTrue))
			Expect(gc.Status.Conditions[1].Reason).Should(Equal(string(apiv1.GatewayClassReasonSupportedVersion)))
			Expect(gc.Status.Conditions[1].Message).Should(ContainSubstring(`supported by kgateway controller`))
		})
	})
})
