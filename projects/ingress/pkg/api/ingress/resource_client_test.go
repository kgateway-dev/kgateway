package ingress_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/test/helpers"
	"github.com/solo-io/solo-kit/test/setup"
	kubev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
)

var _ = Describe("ResourceClient", func() {
	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}
	var (
		namespace string
		cfg       *rest.Config
		ctx       context.Context
		cancel    context.CancelFunc
	)

	BeforeEach(func() {
		namespace = helpers.RandString(8)
		ctx, cancel = context.WithCancel(context.Background())
		var err error
		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		kube, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		_, err = kube.CoreV1().Namespaces().Create(ctx, &kubev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		setup.TeardownKube(namespace)
		cancel()
	})

	It("can CRUD on v1beta1 ingresses", func() {
		kube, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		baseClient := NewResourceClient(kube, &v1.Ingress{})
		ingressClient := v1.NewIngressClientWithBase(baseClient)
		Expect(err).NotTo(HaveOccurred())
		kubeIngressClient := kube.ExtensionsV1beta1().Ingresses(namespace)
		backend := &v1beta1.IngressBackend{
			ServiceName: "foo",
			ServicePort: intstr.IntOrString{
				IntVal: 8080,
			},
		}
		kubeIng, err := kubeIngressClient.Create(ctx, &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rusty",
				Namespace: namespace,
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend,
				TLS: []v1beta1.IngressTLS{
					{
						Hosts:      []string{"some.host"},
						SecretName: "doesntexistanyway",
					},
				},
				Rules: []v1beta1.IngressRule{
					{
						Host: "some.host",
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
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		ingressResource, err := ingressClient.Read(kubeIng.Namespace, kubeIng.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		convertedIng, err := ToKube(ingressResource)
		Expect(err).NotTo(HaveOccurred())
		Expect(convertedIng.Spec).To(Equal(kubeIng.Spec))
	})
})
