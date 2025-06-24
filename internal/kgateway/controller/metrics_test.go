package controller_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

type GinkgoTestReporter struct{}

func (g GinkgoTestReporter) Errorf(format string, args ...interface{}) {
	Fail(fmt.Sprintf(format, args...))
}

func (g GinkgoTestReporter) Fatalf(format string, args ...interface{}) {
	Fail(fmt.Sprintf(format, args...))
}

var _ = Describe("GwControllerMetrics", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	JustBeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		cancel, err = createManager(ctx, inferenceExt, nil)
		Expect(err).NotTo(HaveOccurred())

		ResetMetrics()

	})

	AfterEach(func() {
		cancel()

		// ensure goroutines cleanup
		Eventually(func() bool { return true }).WithTimeout(3 * time.Second).Should(BeTrue())
	})

	It("should generate gateway controller metrics", func() {
		setupGateway(ctx)
		defer deleteGateway(ctx)

		gathered := metricstest.MustGatherMetrics(GinkgoT())

		gathered.AssertMetrics("kgateway_controller_reconciliations_total", []metricstest.ExpectMetric{
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{{Name: "controller", Value: "gateway"}, {Name: "result", Value: "success"}},
				Test:   metricstest.Between(1, 20),
			},
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{{Name: "controller", Value: "gatewayclass"}, {Name: "result", Value: "success"}},
				Test:   metricstest.Between(1, 20),
			},
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{{Name: "controller", Value: "gatewayclass-provisioner"}, {Name: "result", Value: "success"}},
				Test:   metricstest.Between(1, 10),
			},
		})

		gathered.AssertMetrics("kgateway_controller_reconciliations_running", []metricstest.ExpectMetric{
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{{Name: "controller", Value: "gateway"}},
				Test:   metricstest.Between(0, 1),
			},
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{{Name: "controller", Value: "gatewayclass"}},
				Test:   metricstest.Between(0, 1),
			},
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{{Name: "controller", Value: "gatewayclass-provisioner"}},
				Test:   metricstest.Between(0, 1),
			},
		})

		gathered.AssertMetricsLabels("kgateway_controller_reconcile_duration_seconds", [][]metrics.Label{{
			{Name: "controller", Value: "gateway"},
		}, {
			{Name: "controller", Value: "gatewayclass"},
		}, {
			{Name: "controller", Value: "gatewayclass-provisioner"},
		}})
	})

	Context("when metrics are not active", func() {
		BeforeEach(func() {
			metrics.SetActive(false)
		})

		AfterEach(func() {
			metrics.SetActive(true)
		})

		It("should not record metrics if metrics are not active", func() {
			setupGateway(ctx)
			defer deleteGateway(ctx)

			gathered := metricstest.MustGatherMetrics(GinkgoT())

			gathered.AssertMetricNotExists("kgateway_controller_reconciliations_total")
			gathered.AssertMetricNotExists("kgateway_controller_reconciliations_running")
			gathered.AssertMetricNotExists("kgateway_controller_reconcile_duration_seconds")
		})

	})

})

func gateway() *api.Gateway {
	same := api.NamespacesFromSame
	gwName := "gw-" + gatewayClassName + "-metrics"
	gw := api.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwName,
			Namespace: defaultNamespace,
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

	return &gw
}

func deleteGateway(ctx context.Context) {
	gw := gateway()
	err := k8sClient.Delete(ctx, gw)
	Expect(err).NotTo(HaveOccurred())

	// The tests in this suite don't do a good job of cleaning up after themselves, which is relevant because of the shared envtest environment
	// but we can at least that the gateway from this test is deleted
	Eventually(func() bool {
		var createdGateways api.GatewayList
		err := k8sClient.List(ctx, &createdGateways)
		found := false
		for _, foundGw := range createdGateways.Items {
			if foundGw.Name == gw.Name {
				found = true
				break
			}
		}
		return err == nil && !found
	}, timeout, interval).Should(BeTrue(), "gateway not deleted")
}

func setupGateway(ctx context.Context) {
	gw := gateway()
	err := k8sClient.Create(ctx, gw)
	Expect(err).NotTo(HaveOccurred())

	waitForGatewayService(ctx, gw)

	if probs, err := metricstest.GatherAndLint(); err != nil || len(probs) > 0 {
		Fail("metrics linter error: " + err.Error())
	}
}

func waitForGatewayService(ctx context.Context, gw *api.Gateway) corev1.Service {
	var svc corev1.Service

	Eventually(func() bool {
		var createdServices corev1.ServiceList
		err := k8sClient.List(ctx, &createdServices)
		if err != nil {
			return false
		}
		for _, svc = range createdServices.Items {
			if len(svc.ObjectMeta.OwnerReferences) == 1 && svc.ObjectMeta.OwnerReferences[0].UID == gw.GetUID() {
				return true
			}
		}
		return false
	}, timeout, interval).Should(BeTrue(), "service not created")
	Expect(svc.Spec.ClusterIP).NotTo(BeEmpty())

	return svc
}
