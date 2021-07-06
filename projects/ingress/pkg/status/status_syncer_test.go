package status_test

import (
	"context"
	"os"
	"time"

	"github.com/solo-io/gloo/projects/ingress/pkg/translator"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/service"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/gloo/projects/ingress/pkg/status"
	"github.com/solo-io/go-utils/kubeutils"
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

var _ = Describe("StatusSyncer", func() {
	var (
		namespace string
		cfg       *rest.Config
		ctx       context.Context
		cancel    context.CancelFunc

		err           error
		kubeClientset *kubernetes.Clientset
	)

	BeforeEach(func() {
		// https://github.com/solo-io/gloo/issues/3304
		if os.Getenv("RUN_KUBE_TESTS") != "1" {
			Skip("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		}

		ctx, cancel = context.WithCancel(context.Background())
		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		// Initialize the kube Clientset
		kubeClientset, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		// Create test namespace
		namespace = helpers.RandString(8)
		_, err = kubeClientset.CoreV1().Namespaces().Create(&kubev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = setup.TeardownKube(namespace)
		cancel()
	})

	It("updates kube ingresses with endpoints from the service", func() {
		baseIngressClient := ingress.NewResourceClient(kubeClientset, &v1.Ingress{})
		ingressClient := v1.NewIngressClientWithBase(baseIngressClient)
		baseKubeServiceClient := service.NewResourceClient(kubeClientset, &v1.KubeService{})
		kubeServiceClient := v1.NewKubeServiceClientWithBase(baseKubeServiceClient)
		kubeServiceClient = service.NewClientWithSelector(kubeServiceClient, map[string]string{
			"gloo": "ingress-proxy",
		})
		statusEmitter := v1.NewStatusEmitter(kubeServiceClient, ingressClient)
		statusSync := status.NewSyncer(ingressClient)
		statusEventLoop := v1.NewStatusEventLoop(statusEmitter, statusSync)
		statusEventLoopErrs, err := statusEventLoop.Run([]string{namespace}, clients.WatchOpts{Ctx: context.TODO()})
		Expect(err).NotTo(HaveOccurred())
		go func() {
			defer GinkgoRecover()
			err := <-statusEventLoopErrs
			Expect(err).NotTo(HaveOccurred())
		}()

		kubeIngressClient := kubeClientset.ExtensionsV1beta1().Ingresses(namespace)
		backend := &v1beta1.IngressBackend{
			ServiceName: "foo",
			ServicePort: intstr.IntOrString{
				IntVal: 8080,
			},
		}
		kubeIng, err := kubeIngressClient.Create(&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rusty",
				Namespace: namespace,
				Annotations: map[string]string{
					translator.IngressClassKey: "gloo",
				},
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
		})

		kubeSvcClient := kubeClientset.CoreV1().Services(namespace)
		svc_def := kubev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dusty",
				Namespace: namespace,
				Labels: map[string]string{
					"gloo": "ingress-proxy",
				},
			},
			Spec: kubev1.ServiceSpec{
				Selector: map[string]string{
					"gloo": "ingress-proxy",
				},
				Ports: []kubev1.ServicePort{
					{
						Name: "foo",
						Port: 1234,
					},
				},
				Type: kubev1.ServiceTypeLoadBalancer,
			},
			Status: kubev1.ServiceStatus{
				LoadBalancer: kubev1.LoadBalancerStatus{
					Ingress: []kubev1.LoadBalancerIngress{
						{
							Hostname: "hostname",
						},
					},
				},
			},
		}
		svc, err := kubeSvcClient.Create(&svc_def)
		Expect(err).NotTo(HaveOccurred())

		_, err = kubeClientset.CoreV1().Pods(namespace).Create(&kubev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "musty",
				Namespace: namespace,
				Labels: map[string]string{
					"gloo": "ingress-proxy",
				},
			},
			Spec: kubev1.PodSpec{
				Containers: []kubev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			svc, err = kubeSvcClient.Get(svc.Name, metav1.GetOptions{})
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			// kubernetes does set ingress lb, set service status explicitly instead
			svc, err = kubeSvcClient.UpdateStatus(&svc_def)
			Expect(err).NotTo(HaveOccurred())
		}

		Eventually(func() ([]kubev1.LoadBalancerIngress, error) {
			ing, err := kubeIngressClient.Get(kubeIng.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			return ing.Status.LoadBalancer.Ingress, nil
		}, time.Second*10).Should(Equal(svc.Status.LoadBalancer.Ingress))
	})

	It("errors when kube service ExternalName = localhost", func() {
		baseIngressClient := ingress.NewResourceClient(kubeClientset, &v1.Ingress{})
		ingressClient := v1.NewIngressClientWithBase(baseIngressClient)
		baseKubeServiceClient := service.NewResourceClient(kubeClientset, &v1.KubeService{})
		kubeServiceClient := v1.NewKubeServiceClientWithBase(baseKubeServiceClient)
		kubeServiceClient = service.NewClientWithSelector(kubeServiceClient, map[string]string{
			"gloo": "ingress-proxy",
		})
		statusEmitter := v1.NewStatusEmitter(kubeServiceClient, ingressClient)
		statusSync := status.NewSyncer(ingressClient)
		statusEventLoop := v1.NewStatusEventLoop(statusEmitter, statusSync)
		statusEventLoopErrs, err := statusEventLoop.Run([]string{namespace}, clients.WatchOpts{Ctx: context.TODO()})
		Expect(err).NotTo(HaveOccurred())
		go func() {
			defer GinkgoRecover()
			err := <-statusEventLoopErrs
			// Expect an error to have occurred during the statusEventLoop
			Expect(err).Should(MatchError(ContainSubstring("Invalid attempt to use localhost name")))
		}()

		backend := &v1beta1.IngressBackend{
			ServiceName: "foo",
			ServicePort: intstr.IntOrString{
				IntVal: 8080,
			},
		}

		kubeIngressClient := kubeClientset.ExtensionsV1beta1().Ingresses(namespace)
		kubeIngress, err := kubeIngressClient.Create(&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rusty",
				Namespace: namespace,
				Annotations: map[string]string{
					translator.IngressClassKey: "gloo",
				},
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
		})

		kubeSvcClient := kubeClientset.CoreV1().Services(namespace)
		kubeSvcDefinition := kubev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dusty",
				Namespace: namespace,
				Labels: map[string]string{
					"gloo": "ingress-proxy",
				},
			},
			Spec: kubev1.ServiceSpec{
				Selector: map[string]string{
					"gloo": "ingress-proxy",
				},
				Type:         kubev1.ServiceTypeExternalName,
				ExternalName: "localhost", // this should not be allowed
			},
			Status: kubev1.ServiceStatus{
				LoadBalancer: kubev1.LoadBalancerStatus{
					Ingress: []kubev1.LoadBalancerIngress{
						{
							Hostname: "hostname",
						},
					},
				},
			},
		}
		kubeSvc, err := kubeSvcClient.Create(&kubeSvcDefinition)
		Expect(err).NotTo(HaveOccurred())

		_, err = kubeClientset.CoreV1().Pods(namespace).Create(&kubev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "musty",
				Namespace: namespace,
				Labels: map[string]string{
					"gloo": "ingress-proxy",
				},
			},
			Spec: kubev1.PodSpec{
				Containers: []kubev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			kubeSvc, err = kubeSvcClient.Get(kubeSvc.Name, metav1.GetOptions{})
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		if len(kubeSvc.Status.LoadBalancer.Ingress) == 0 {
			// kubernetes does set ingress lb, set service status explicitly instead
			kubeSvc, err = kubeSvcClient.UpdateStatus(&kubeSvcDefinition)
			Expect(err).NotTo(HaveOccurred())
		}

		// The only service that we have configured should be rejected
		Eventually(func() ([]kubev1.LoadBalancerIngress, error) {
			ing, err := kubeIngressClient.Get(kubeIngress.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			return ing.Status.LoadBalancer.Ingress, nil
		}, time.Second*10).Should(BeEmpty())
	})
})
