package gloo_test

import (
	kubev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/service"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/test/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

// Kubernetes tests for resource client from projects/ingress/pkg/api/service
var _ = Describe("ResourceClient", func() {

	var (
		testNamespace string

		kubeClient kubernetes.Interface
	)

	BeforeEach(func() {
		var err error

		testNamespace = helpers.RandString(8)
		kubeClient = resourceClientset.KubeClients()

		_, err = kubeClient.CoreV1().Namespaces().Create(ctx, &kubev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err := kubeClient.CoreV1().Namespaces().Delete(ctx, testNamespace, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("can CRUD on v1 Services", func() {
		baseClient := service.NewResourceClient(kubeClient, &v1.Ingress{})
		svcClient := v1.NewKubeServiceClientWithBase(baseClient)

		kubeSvcClient := kubeClient.CoreV1().Services(testNamespace)
		kubeSvc, err := kubeSvcClient.Create(ctx, &kubev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hi",
				Namespace: testNamespace,
			},
			Spec: kubev1.ServiceSpec{
				Ports: []kubev1.ServicePort{
					{
						Name:     "http",
						Protocol: kubev1.ProtocolTCP,
						Port:     1234,
					},
				},
				Selector: map[string]string{"hi": "bye"},
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		ingressResource, err := svcClient.Read(kubeSvc.Namespace, kubeSvc.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		convertedIng, err := service.ToKube(ingressResource)
		Expect(err).NotTo(HaveOccurred())
		Expect(convertedIng.Spec).To(Equal(kubeSvc.Spec))
	})
})
