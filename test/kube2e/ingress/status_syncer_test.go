package ingress_test

import (
	"context"
	"time"

	"github.com/solo-io/gloo/projects/ingress/pkg/translator"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/service"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/gloo/projects/ingress/pkg/status"
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

var _ = Describe("StatusSyncer", func() {

	Context("ClusterIngress", func() {
		// Copied from: https://github.com/solo-io/gloo/blob/52e15bb135c6ae51fae21f0b1187943b77981b7d/projects/clusteringress/pkg/status/status_syncer_test.go#L28

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
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "gloo",
					},
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

			kubeSvcClient := kube.CoreV1().Services(namespace)
			svc, err := kubeSvcClient.Create(ctx, &kubev1.Service{
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
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = kube.CoreV1().Pods(namespace).Create(ctx, &kubev1.Pod{
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
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(time.Second) // give the kube service time to update lb endpoints
			svc, err = kubeSvcClient.Get(ctx, svc.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			// note (ilackarms): unless running on a cloud provider that supports
			// kube lb ingress, the status ips for the service and ingress will be empty
			Eventually(func() ([]kubev1.LoadBalancerIngress, error) {
				ing, err := kubeIngressClient.Get(ctx, kubeIng.Name, metav1.GetOptions{})
				if err != nil {
					return nil, err
				}
				return ing.Status.LoadBalancer.Ingress, nil
			}, time.Second*10).Should(Equal(svc.Status.LoadBalancer.Ingress))
		})

	})

	Context("Ingress", func() {
		// Copied from: https://github.com/solo-io/gloo/blob/52e15bb135c6ae51fae21f0b1187943b77981b7d/projects/ingress/pkg/status/status_syncer_test.go#L29

		var (
			namespace string
			cfg       *rest.Config
			ctx       context.Context
			cancel    context.CancelFunc

			err           error
			kubeClientset *kubernetes.Clientset
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			cfg, err = kubeutils.GetConfig("", "")
			Expect(err).NotTo(HaveOccurred())

			// Initialize the kube Clientset
			kubeClientset, err = kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create test namespace
			namespace = helpers.RandString(8)
			_, err = kubeClientset.CoreV1().Namespaces().Create(ctx, &kubev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}, metav1.CreateOptions{})
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

			kubeIngressClient := kubeClientset.NetworkingV1().Ingresses(namespace)
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
					Annotations: map[string]string{
						translator.IngressClassKey: "gloo",
					},
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
			svc, err := kubeSvcClient.Create(ctx, &svc_def, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = kubeClientset.CoreV1().Pods(namespace).Create(ctx, &kubev1.Pod{
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
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				svc, err = kubeSvcClient.Get(ctx, svc.Name, metav1.GetOptions{})
				return err
			}, time.Second*10).ShouldNot(HaveOccurred())

			if len(svc.Status.LoadBalancer.Ingress) == 0 {
				// kubernetes does set ingress lb, set service status explicitly instead
				svc, err = kubeSvcClient.UpdateStatus(ctx, &svc_def, metav1.UpdateOptions{})
				Expect(err).NotTo(HaveOccurred())
			}

			Eventually(func() ([]kubev1.LoadBalancerIngress, error) {
				ing, err := kubeIngressClient.Get(ctx, kubeIng.Name, metav1.GetOptions{})
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

			backend := &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "foo",
					Port: networkingv1.ServiceBackendPort{
						Number: 8080,
					},
				},
			}

			kubeIngressClient := kubeClientset.NetworkingV1().Ingresses(namespace)
			pathType := networkingv1.PathTypeImplementationSpecific
			kubeIngress, err := kubeIngressClient.Create(ctx, &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rusty",
					Namespace: namespace,
					Annotations: map[string]string{
						translator.IngressClassKey: "gloo",
					},
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
			kubeSvc, err := kubeSvcClient.Create(ctx, &kubeSvcDefinition, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = kubeClientset.CoreV1().Pods(namespace).Create(ctx, &kubev1.Pod{
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
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				kubeSvc, err = kubeSvcClient.Get(ctx, kubeSvc.Name, metav1.GetOptions{})
				return err
			}, time.Second*10).ShouldNot(HaveOccurred())

			if len(kubeSvc.Status.LoadBalancer.Ingress) == 0 {
				// kubernetes does set ingress lb, set service status explicitly instead
				kubeSvc, err = kubeSvcClient.UpdateStatus(ctx, &kubeSvcDefinition, metav1.UpdateOptions{})
				Expect(err).NotTo(HaveOccurred())
			}

			// The only service that we have configured should be rejected
			Eventually(func() ([]kubev1.LoadBalancerIngress, error) {
				ing, err := kubeIngressClient.Get(ctx, kubeIngress.Name, metav1.GetOptions{})
				if err != nil {
					return nil, err
				}
				return ing.Status.LoadBalancer.Ingress, nil
			}, time.Second*10).Should(BeEmpty())
		})
	})

})
