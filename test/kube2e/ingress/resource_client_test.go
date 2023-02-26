package ingress_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/test/helpers"
	"github.com/solo-io/solo-kit/test/setup"
	kubev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
)

var _ = Describe("ResourceClient", func() {

	Context("Ingress", func() {
		// Copied from: https://github.com/solo-io/gloo/blob/52e15bb135c6ae51fae21f0b1187943b77981b7d/projects/ingress/pkg/api/ingress/resource_client_test.go#L25

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

		It("can CRUD on v1 ingresses", func() {
			kube, err := kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())
			baseClient := NewResourceClient(kube, &v1.Ingress{})
			ingressClient := v1.NewIngressClientWithBase(baseClient)
			Expect(err).NotTo(HaveOccurred())
			kubeIngressClient := kube.NetworkingV1().Ingresses(namespace)
			backend := &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "foo",
					Port: networkingv1.ServiceBackendPort{
						Number: 8080,
					},
				},
			}
			pathType := networkingv1.PathTypeImplementationSpecific
			kubeIng, err := kubeIngressClient.Create(ctx, &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rusty",
					Namespace: namespace,
				},
				Spec: networkingv1.IngressSpec{
					DefaultBackend: backend,
					TLS: []networkingv1.IngressTLS{
						{
							Hosts:      []string{"some.host"},
							SecretName: "doesntexistanyway",
						},
					},
					Rules: []networkingv1.IngressRule{
						{
							Host: "some.host",
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
			ingressResource, err := ingressClient.Read(kubeIng.Namespace, kubeIng.Name, clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())
			convertedIng, err := ToKube(ingressResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(convertedIng.Spec).To(Equal(kubeIng.Spec))
		})

	})

})
