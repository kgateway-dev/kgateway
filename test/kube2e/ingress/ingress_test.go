package ingress_test

import (
	"context"
	"time"

	"github.com/solo-io/gloo/test/kube2e"

	"github.com/solo-io/k8s-utils/testutils/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/k8s-utils/kubeutils"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Kube2e: Ingress", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() { cancel() })

	It("works", func() {
		cfg, err := kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		kube, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		kubeIngressClient := kube.NetworkingV1().Ingresses(testHelper.InstallNamespace)

		backend := &networkingv1.IngressBackend{
			Service: &networkingv1.IngressServiceBackend{
				Name: helper.TestrunnerName,
				Port: networkingv1.ServiceBackendPort{
					Number: helper.TestRunnerPort,
				},
			},
		}
		pathType := networkingv1.PathTypeImplementationSpecific
		kubeIng, err := kubeIngressClient.Create(ctx, &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "simple-ingress-route",
				Namespace:   testHelper.InstallNamespace,
				Annotations: map[string]string{"kubernetes.io/ingress.class": "gloo"},
			},
			Spec: networkingv1.IngressSpec{
				DefaultBackend: backend,
				Rules: []networkingv1.IngressRule{
					{
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										PathType: &pathType,
										Backend:  *backend,
									},
								},
							},
						},
					},
				},
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(kubeIng).NotTo(BeNil())

		ingressProxy := "ingress-proxy"
		ingressPort := 80
		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
			Protocol:          "http",
			Path:              "/",
			Method:            "GET",
			Host:              ingressProxy,
			Service:           ingressProxy,
			Port:              ingressPort,
			ConnectionTimeout: 1,
		}, kube2e.GetSimpleTestRunnerHttpResponse(), 1, time.Minute*2, 1*time.Second)
	})
})
