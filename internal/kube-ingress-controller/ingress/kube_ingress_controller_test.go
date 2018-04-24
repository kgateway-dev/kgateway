package ingress

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	kubeplugin "github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/crd"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("KubeIngressController", func() {
	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}
	var (
		masterUrl, kubeconfigPath string
		namespace                 string
	)
	BeforeEach(func() {
		namespace = RandString(8)
		err := SetupKubeForTest(namespace)
		Must(err)
		kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		masterUrl = ""
	})
	AfterEach(func() {
		TeardownKube(namespace)
	})
	Describe("controller", func() {
		var (
			ingressCtl *IngressController
			kubeClient kubernetes.Interface
			glooClient storage.Interface
		)
		BeforeEach(func() {
			cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
			Must(err)

			glooClient, err = crd.NewStorage(cfg, namespace, time.Second)
			Must(err)

			ingressCtl, err = NewIngressController(cfg, glooClient, time.Second, true)
			Must(err)

			go ingressCtl.Run(make(chan struct{}))

			kubeClient, err = kubernetes.NewForConfig(cfg)
			Must(err)
		})
		Context("an ingress is created without our ingress class", func() {
			var (
				createdIngress *v1beta1.Ingress
				err            error
			)
			BeforeEach(func() {
				serviceName := "somethingsomethingsomething"
				servicePort := intstr.FromInt(8080)

				// add an ingress
				ingress := &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "ingress-",
						Namespace:    namespace,
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: serviceName,
							ServicePort: servicePort,
						},
					},
				}
				createdIngress, err = kubeClient.ExtensionsV1beta1().Ingresses(namespace).Create(ingress)
				Must(err)
			})
			AfterEach(func() {
				err = kubeClient.ExtensionsV1beta1().Ingresses(namespace).Delete(createdIngress.Name, nil)
				Must(err)
				time.Sleep(time.Second)
			})
			It("does not return an error", func() {
				select {
				case <-time.After(time.Second):
					// passed without error
				case err := <-ingressCtl.Error():
					Expect(err).NotTo(HaveOccurred())
					Fail("err passed, but was nil")
				}
			})
			It("ignores the ingress", func() {
				upstreams, err := glooClient.V1().Upstreams().List()
				Expect(err).NotTo(HaveOccurred())
				Expect(upstreams).To(HaveLen(0))
				virtualServiceList, err := glooClient.V1().VirtualServices().List()
				Expect(err).NotTo(HaveOccurred())
				Expect(virtualServiceList).To(HaveLen(0))
			})
		})
		Context("an ingress is created with a default backend", func() {
			var (
				createdIngress *v1beta1.Ingress
				err            error
				serviceName    = "somethingsomethingsomething"
				servicePort    = intstr.FromInt(8080)
				defaultBackend = &v1beta1.IngressBackend{
					ServiceName: serviceName,
					ServicePort: servicePort,
				}
			)
			BeforeEach(func() {

				// add an ingress
				ingress := &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "ingress-",
						Namespace:    namespace,
						Annotations:  map[string]string{"kubernetes.io/ingress.class": GlooIngressClass},
					},
					Spec: v1beta1.IngressSpec{
						Backend: defaultBackend,
					},
				}
				createdIngress, err = kubeClient.ExtensionsV1beta1().Ingresses(namespace).Create(ingress)
				Must(err)
				time.Sleep(time.Second)
			})
			AfterEach(func() {
				err = kubeClient.ExtensionsV1beta1().Ingresses(namespace).Delete(createdIngress.Name, nil)
				Must(err)
				time.Sleep(time.Second)
			})
			It("does not return an error", func() {
				select {
				case <-time.After(time.Second * 1):
					// passed without error
				case err := <-ingressCtl.Error():
					Expect(err).NotTo(HaveOccurred())
					Fail("err passed, but was nil")
				}
			})
			It("creates the default virtualservice for the ingress", func() {
				glooRoute := &v1.Route{
					Matcher: &v1.Route_RequestMatcher{
						RequestMatcher: &v1.RequestMatcher{
							Path: &v1.RequestMatcher_PathPrefix{
								PathPrefix: "/",
							},
						},
					},
					SingleDestination: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &v1.UpstreamDestination{
								Name: UpstreamName(createdIngress.Namespace, createdIngress.Spec.Backend.ServiceName, createdIngress.Spec.Backend.ServicePort),
							},
						},
					},
				}
				glooVirtualService := &v1.VirtualService{
					Name:    defaultVirtualService,
					Domains: []string{"*"},
					Routes:  []*v1.Route{glooRoute},
				}
				virtualServiceList, err := glooClient.V1().VirtualServices().List()
				Expect(err).NotTo(HaveOccurred())
				Expect(virtualServiceList).To(HaveLen(1))
				virtualService := virtualServiceList[0]
				Expect(virtualService.Name).To(Equal(defaultVirtualService))
				// have to set metadata
				glooVirtualService.Metadata = virtualService.Metadata
				Expect(virtualService).To(Equal(glooVirtualService))
			})
			It("creates an upstream for the ingress backend", func() {
				expectedUpstream := ingressCtl.newUpstreamFromBackend(namespace, *defaultBackend)
				upstreamList, err := glooClient.V1().Upstreams().List()
				Expect(err).NotTo(HaveOccurred())
				Expect(upstreamList).To(HaveLen(1))
				actualUpstream := upstreamList[0]
				// have to set metadata
				expectedUpstream.Metadata = actualUpstream.Metadata
				Expect(actualUpstream).To(Equal(expectedUpstream))
			})
		})
		Context("an ingress is created with multiple rules", func() {
			var (
				createdIngress *v1beta1.Ingress
				err            error
			)
			BeforeEach(func() {
				// add an ingress
				ingress := &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "ingress-",
						Namespace:    namespace,
						Annotations:  map[string]string{"kubernetes.io/ingress.class": GlooIngressClass},
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{
							{
								Hosts:      []string{"host1"},
								SecretName: "my-secret-1",
							},
							{
								Hosts:      []string{"host2"},
								SecretName: "my-secret-2",
							},
						},
						Rules: []v1beta1.IngressRule{
							{
								Host: "host1",
								IngressRuleValue: v1beta1.IngressRuleValue{
									HTTP: &v1beta1.HTTPIngressRuleValue{
										Paths: []v1beta1.HTTPIngressPath{
											{
												Path: "/foo/bar",
												Backend: v1beta1.IngressBackend{
													ServiceName: "service1",
													ServicePort: intstr.FromInt(1234),
												},
											},
											{
												Path: "/foo/baz",
												Backend: v1beta1.IngressBackend{
													ServiceName: "service2",
													ServicePort: intstr.FromInt(3456),
												},
											},
										},
									},
								},
							},
							{
								Host: "host2",
								IngressRuleValue: v1beta1.IngressRuleValue{
									HTTP: &v1beta1.HTTPIngressRuleValue{
										Paths: []v1beta1.HTTPIngressPath{
											{
												Path: "/foo/bar",
												Backend: v1beta1.IngressBackend{
													ServiceName: "service3",
													ServicePort: intstr.FromInt(1234),
												},
											},
											{
												Path: "/straw/berry",
												Backend: v1beta1.IngressBackend{
													ServiceName: "service4",
													ServicePort: intstr.FromString("foo"),
												},
											},
											{
												Path: "/bat/girl",
												Backend: v1beta1.IngressBackend{
													ServiceName: "service4",
													ServicePort: intstr.FromString("foo"),
												},
											},
										},
									},
								},
							},
						},
					},
				}
				createdIngress, err = kubeClient.ExtensionsV1beta1().Ingresses(namespace).Create(ingress)
				Must(err)
				time.Sleep(time.Second * 2)
			})
			AfterEach(func() {
				err = kubeClient.ExtensionsV1beta1().Ingresses(namespace).Delete(createdIngress.Name, nil)
				Must(err)
			})
			It("does not return an error", func() {
				select {
				case <-time.After(time.Second * 2):
					// passed without error
				case err := <-ingressCtl.Error():
					Expect(err).NotTo(HaveOccurred())
					Fail("err passed, but was nil")
				}
			})
			It("should de-duplicate repeated upstreams", func() {
				time.Sleep(time.Second * 3)
				expectedUpstreams := make(map[string]*v1.Upstream)
				for _, rule := range createdIngress.Spec.Rules {
					for _, path := range rule.HTTP.Paths {
						upstreamName := UpstreamName(createdIngress.Namespace, path.Backend.ServiceName, path.Backend.ServicePort)
						expectedUpstreams[upstreamName] = &v1.Upstream{
							Name: upstreamName,
							Type: kubeplugin.UpstreamTypeKube,
							Spec: kubeplugin.EncodeUpstreamSpec(kubeplugin.UpstreamSpec{
								ServiceName:      path.Backend.ServiceName,
								ServiceNamespace: namespace,
								ServicePort:      path.Backend.ServicePort.IntVal,
							}),
						}
					}
				}
				upstreams, err := glooClient.V1().Upstreams().List()
				Expect(err).NotTo(HaveOccurred())
				Expect(upstreams).To(HaveLen(len(expectedUpstreams)))
				for _, us := range upstreams {
					Expect(expectedUpstreams).To(HaveKey(us.Name))
					// ignore metadata, as teh client sets this
					us.Metadata = nil
					Expect(expectedUpstreams[us.Name]).To(Equal(us))
				}
			})
			It("create a route for every path", func() {
				time.Sleep(4 * time.Second)
				expectedVirtualServices := map[string]*v1.VirtualService{
					"host2": {
						Name:    "host2",
						Domains: []string{"host2"},
					},
					"host1": {
						Name:    "host1",
						Domains: []string{"host1"},
					},
				}

				for _, rule := range createdIngress.Spec.Rules {
					for _, path := range rule.HTTP.Paths {
						vService := expectedVirtualServices[rule.Host]
						vService.Routes = append(vService.Routes, &v1.Route{
							Matcher: &v1.Route_RequestMatcher{
								RequestMatcher: &v1.RequestMatcher{
									Path: &v1.RequestMatcher_PathRegex{PathRegex: path.Path},
								},
							},
							SingleDestination: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: &v1.UpstreamDestination{
										Name: UpstreamName(createdIngress.Namespace, path.Backend.ServiceName, path.Backend.ServicePort),
									},
								},
							},
						})
						sortRoutes(vService.Routes)
						expectedVirtualServices[rule.Host] = vService
					}
				}

				for _, tls := range createdIngress.Spec.TLS {
					for _, host := range tls.Hosts {
						vService := expectedVirtualServices[host]
						vService.SslConfig = &v1.SSLConfig{
							SecretRef: tls.SecretName,
						}
						expectedVirtualServices[host] = vService
					}
				}
				virtuavirtualServiceList, err := glooClient.V1().VirtualServices().List()
				Expect(err).NotTo(HaveOccurred())
				Expect(virtuavirtualServiceList).To(HaveLen(len(expectedVirtualServices)))
				for _, virtualService := range virtuavirtualServiceList {
					// ignore metadata
					virtualService.Metadata = nil
					Expect(expectedVirtualServices).To(ContainElement(virtualService))
				}
			})
		})
	})
})
