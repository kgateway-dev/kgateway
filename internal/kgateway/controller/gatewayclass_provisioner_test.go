package controller_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	timeout  = time.Second * 10
	interval = time.Millisecond * 250
)

var _ = Describe("GatewayClassProvisioner", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		cancel, err = createManager(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		// ensure goroutines cleanup
		Eventually(func() bool { return true }).WithTimeout(3 * time.Second).Should(BeTrue())
	})

	Context("create", func() {
		It("should create the default GCs", func() {
			Eventually(func() bool {
				gcs := &apiv1.GatewayClassList{}
				err := k8sClient.List(ctx, gcs)
				if err != nil {
					return false
				}
				if len(gcs.Items) != 2 {
					return false
				}
				for _, gc := range gcs.Items {
					if !gwClasses.Has(gc.Name) {
						return false
					}
				}
				return true
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("delete", func() {
		AfterEach(func() {
			Eventually(func() bool {
				gcs := &apiv1.GatewayClassList{}
				err := k8sClient.List(ctx, gcs)
				return err == nil && len(gcs.Items) == 2
			}, timeout, interval).Should(BeTrue())
		})
		It("should be recreated", func() {
			By("deleting the default GCs")
			for name := range gwClasses {
				err := k8sClient.Delete(ctx, &apiv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: name}})
				Expect(err).NotTo(HaveOccurred())
			}
			By("waiting for the GCs to be recreated")
			Eventually(func() bool {
				gcs := &apiv1.GatewayClassList{}
				err := k8sClient.List(ctx, gcs)
				if err != nil {
					return false
				}
				GinkgoWriter.Println("gcs.Items", len(gcs.Items))
				return len(gcs.Items) == 2
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("update", func() {
		var (
			description string
		)
		BeforeEach(func() {
			By("getting the default GC")
			gc := &apiv1.GatewayClass{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gc)
			Expect(err).NotTo(HaveOccurred())
			description = *gc.Spec.Description
		})
		AfterEach(func() {
			By("restoring the default GC value")
			gc := &apiv1.GatewayClass{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gc)
			Expect(err).NotTo(HaveOccurred())
			gc.Spec.Description = ptr.To(description)
			err = k8sClient.Update(ctx, gc)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should not be overwritten", func() {
			By("updating a default GC")
			var gc *apiv1.GatewayClass
			Eventually(func() bool {
				gc = &apiv1.GatewayClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gc)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("updating the GC")
			gc.Spec.Description = ptr.To("updated")
			err := k8sClient.Update(ctx, gc)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the GC to be updated")
			Eventually(func() bool {
				gc = &apiv1.GatewayClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gc)
				if err != nil {
					return false
				}
				if gc.Spec.Description == nil {
					return false
				}
				return *gc.Spec.Description == "updated"
			}, timeout, interval).Should(BeTrue())
		})
	})
})
