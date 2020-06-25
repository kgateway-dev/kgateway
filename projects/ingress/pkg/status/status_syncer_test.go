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
	)

	BeforeEach(func() {
		if os.Getenv("RUN_KUBE_TESTS") != "1" {
			Skip("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		}
		namespace = helpers.RandString(8)
		var err error
		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		kube, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		_, err = kube.CoreV1().Namespaces().Create(&kubev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).NotTo(HaveOccurred())

	})
	AfterEach(func() {
		setup.TeardownKube(namespace)
	})

	It("updates kube ingresses with endpoints from the service", func() {
		kube, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		baseIngressClient := ingress.NewResourceClient(kube, &v1.Ingress{})
		ingressClient := v1.NewIngressClientWithBase(baseIngressClient)
		baseKubeServiceClient := service.NewResourceClient(kube, &v1.KubeService{})
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

		kubeIngressClient := kube.ExtensionsV1beta1().Ingresses(namespace)
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

		kubeSvcClient := kube.CoreV1().Services(namespace)
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

		_, err = kube.CoreV1().Pods(namespace).Create(&kubev1.Pod{
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

		time.Sleep(time.Second) // give the kube service time to update lb endpoints
		svc, err = kubeSvcClient.Get(svc.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

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
})
