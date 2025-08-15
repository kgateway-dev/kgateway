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

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/assertions"
)

const (
	timeout  = time.Second * 10
	interval = time.Millisecond * 250
)

var _ = Describe("GatewayClassProvisioner", func() {
	var (
		ctx              context.Context
		cancel           context.CancelFunc
		goroutineMonitor *assertions.GoRoutineMonitor
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		goroutineMonitor = assertions.NewGoRoutineMonitor()
	})

	AfterEach(func() {
		cancel()
		waitForGoroutinesToFinish(goroutineMonitor)
	})

	When("no GatewayClasses exist on the cluster", func() {
		BeforeEach(func() {
			var err error
			cancel, err = createManager(ctx, nil, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the default GCs", func() {
			Eventually(func() bool {
				gcs := &apiv1.GatewayClassList{}
				err := k8sClient.List(ctx, gcs)
				if err != nil {
					return false
				}
				if len(gcs.Items) != gwClasses.Len() {
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

	When("existing GatewayClasses from other controllers exist on the cluster", func() {
		var (
			otherGC           *apiv1.GatewayClass
			wrongControllerGC *apiv1.GatewayClass
		)
		BeforeEach(func() {
			// Create GatewayClass owned by another controller
			otherGC = &apiv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other-controller",
				},
				Spec: apiv1.GatewayClassSpec{
					ControllerName: "other.controller/name",
				},
			}
			Expect(k8sClient.Create(ctx, otherGC)).To(Succeed())

			// Create our GatewayClass but with wrong controller
			wrongControllerGC = &apiv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "wrong-controller",
				},
				Spec: apiv1.GatewayClassSpec{
					ControllerName: "wrong.controller/name",
				},
			}
			Expect(k8sClient.Create(ctx, wrongControllerGC)).To(Succeed())

			var err error
			cancel, err = createManager(ctx, nil, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// Cleanup the test GatewayClasses
			Expect(k8sClient.Delete(ctx, otherGC)).To(Succeed())
			Expect(k8sClient.Delete(ctx, wrongControllerGC)).To(Succeed())
		})

		It("should create our GCs and not affect others", func() {
			By("verifying our GatewayClasses are created with correct controller")
			Eventually(func() bool {
				for className := range gwClasses {
					gc := &apiv1.GatewayClass{}
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: className}, gc); err != nil {
						return false
					}
					if gc.Spec.ControllerName != apiv1.GatewayController(gatewayControllerName) {
						return false
					}
				}
				return true
			}, timeout, interval).Should(BeTrue())
		})
	})

	When("the default GCs are deleted", func() {
		BeforeEach(func() {
			var err error
			cancel, err = createManager(ctx, nil, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Eventually(func() bool {
				gcs := &apiv1.GatewayClassList{}
				err := k8sClient.List(ctx, gcs)
				return err == nil && len(gcs.Items) == gwClasses.Len()
			}, timeout, interval).Should(BeTrue())
		})

		It("should be recreated by the provisioner", func() {
			By("deleting the default GCs")

			// wait for the default GCs to be created, especially needed if this is the first test to run
			gc := &apiv1.GatewayClass{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gc)
			}, timeout, interval).Should(Succeed())

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
				return len(gcs.Items) == gwClasses.Len()
			}, timeout, interval).Should(BeTrue())
		})
	})

	When("a default GC is updated", func() {
		var (
			description string
		)
		BeforeEach(func() {
			var err error
			cancel, err = createManager(ctx, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			By("getting the default GC")
			gc := &apiv1.GatewayClass{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gc)
			}, timeout, interval).Should(Succeed())
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

		It("should not be overwritten by the provisioner", func() {
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

	When("custom GatewayClass configurations are provided", func() {
		var customClassConfigs map[string]*controller.ClassInfo

		BeforeEach(func() {
			customClassConfigs = map[string]*controller.ClassInfo{
				"custom-class": {
					Description: "custom gateway class",
					Labels: map[string]string{
						"custom": "true",
					},
					Annotations: map[string]string{
						"custom.annotation": "value",
					},
				},
			}

			var err error
			cancel, err = createManager(ctx, nil, customClassConfigs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create GatewayClasses with custom configurations", func() {
			Eventually(func() bool {
				gc := &apiv1.GatewayClass{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "custom-class"}, gc); err != nil {
					return false
				}
				return gc.Spec.ControllerName == apiv1.GatewayController(gatewayControllerName) &&
					*gc.Spec.Description == "custom gateway class" &&
					gc.Labels["custom"] == "true" &&
					gc.Annotations["custom.annotation"] == "value"
			}, timeout, interval).Should(BeTrue())
		})
	})
})
