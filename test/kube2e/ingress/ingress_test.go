package ingress_test

import (
	"time"

	"github.com/solo-io/go-utils/testutils/helper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/kubeutils"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Kube2e: Ingress", func() {

	It("works", func() {
		cfg, err := kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		kube, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		kubeIngressClient := kube.ExtensionsV1beta1().Ingresses(testHelper.InstallNamespace)

		backend := &v1beta1.IngressBackend{
			ServiceName: helper.TestrunnerName,
			ServicePort: intstr.IntOrString{
				IntVal: helper.TestRunnerPort,
			},
		}
		kubeIng, err := kubeIngressClient.Create(&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "simple-ingress-route",
				Namespace:   testHelper.InstallNamespace,
				Annotations: map[string]string{"kubernetes.io/ingress.class": "gloo"},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend,
				Rules: []v1beta1.IngressRule{
					{
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{
									{
										Backend: *backend,
									},
								},
							},
						},
					},
				},
			},
		})
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
		}, helper.SimpleHttpResponse, 1, time.Minute*2, 1*time.Second)
	})
})
