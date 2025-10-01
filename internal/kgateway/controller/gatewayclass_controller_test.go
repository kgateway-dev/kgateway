package controller_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
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
			Eventually(func() (*metav1.Condition, error) {
				gc := &apiv1.GatewayClass{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gc); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(gc.Status.Conditions, string(apiv1.GatewayClassConditionStatusAccepted)), nil
			}, timeout, interval).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(string(apiv1.GatewayClassConditionStatusAccepted))),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(string(apiv1.GatewayClassReasonAccepted))),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring(`accepted by kgateway controller`)),
			))
		})
	})
})
