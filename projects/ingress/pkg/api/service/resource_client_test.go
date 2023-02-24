package service_test

import (
	"context"

	"github.com/solo-io/gloo/test/testutils"
	kubev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/ingress/pkg/api/service"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/test/helpers"
	"github.com/solo-io/solo-kit/test/setup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
)

var _ = Describe("ResourceClient", func() {
	if !testutils.ShouldRunKubeTests() {
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
		var err error
		ctx, cancel = context.WithCancel(context.Background())
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

	It("can CRUD on v1 Services", func() {
		kube, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		baseClient := NewResourceClient(kube, &v1.Ingress{})
		svcClient := v1.NewKubeServiceClientWithBase(baseClient)
		Expect(err).NotTo(HaveOccurred())
		kubeSvcClient := kube.CoreV1().Services(namespace)
		kubeSvc, err := kubeSvcClient.Create(ctx, &kubev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hi",
				Namespace: namespace,
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
		convertedIng, err := ToKube(ingressResource)
		Expect(err).NotTo(HaveOccurred())
		Expect(convertedIng.Spec).To(Equal(kubeSvc.Spec))
	})
})
