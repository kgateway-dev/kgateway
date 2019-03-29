package gateway_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/solo-io/go-utils/testutils/helper"

	"k8s.io/client-go/kubernetes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	grpcv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/test/setup"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	gloov1plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	corev1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

var _ = Describe("Kube2e: gateway", func() {

	var (
		ctx        context.Context
		cancel     context.CancelFunc
		cfg        *rest.Config
		kubeClient kubernetes.Interface

		gatewayClient        v1.GatewayClient
		virtualServiceClient v1.VirtualServiceClient
		upstreamGroupClient  gloov1.UpstreamGroupClient
		upstreamClient       gloov1.UpstreamClient
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		kubeClient, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		cache := kube.NewKubeCache(ctx)
		gatewayClientFactory := &factory.KubeResourceClientFactory{
			Crd:         v1.GatewayCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		virtualServiceClientFactory := &factory.KubeResourceClientFactory{
			Crd:         v1.VirtualServiceCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}

		upstreamGroupClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.UpstreamGroupCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}

		upstreamClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.UpstreamCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}

		gatewayClient, err = v1.NewGatewayClient(gatewayClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = gatewayClient.Register()
		Expect(err).NotTo(HaveOccurred())

		virtualServiceClient, err = v1.NewVirtualServiceClient(virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualServiceClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamGroupClient, err = gloov1.NewUpstreamGroupClient(upstreamGroupClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamGroupClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamClient, err = gloov1.NewUpstreamClient(upstreamClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamClient.Register()
		Expect(err).NotTo(HaveOccurred())

	})

	Context("tests with virtual service", func() {

		AfterEach(func() {
			cancel()
			err := virtualServiceClient.Delete(testHelper.InstallNamespace, "vs", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("works", func() {

			_, err := virtualServiceClient.Write(&v1.VirtualService{

				Metadata: core.Metadata{
					Name:      "vs",
					Namespace: testHelper.InstallNamespace,
				},
				VirtualHost: &gloov1.VirtualHost{
					Name:    "default",
					Domains: []string{"*"},
					Routes: []*gloov1.Route{{
						Matcher: &gloov1.Matcher{
							PathSpecifier: &gloov1.Matcher_Prefix{
								Prefix: "/",
							},
						},
						Action: &gloov1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										Upstream: core.ResourceRef{
											Namespace: testHelper.InstallNamespace,
											Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, "testrunner", helper.TestRunnerPort)},
									},
								},
							},
						},
					}},
				},
			}, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
			// wait for default gateway to be created
			Eventually(func() (*v1.Gateway, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			}, "15s", "0.5s").Should(Not(BeNil()))

			gatewayProxy := "gateway-proxy"
			gatewayPort := int(80)
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 10, // this is important, as sometimes curl hangs
			}, helper.SimpleHttpResponse, 1, 120*time.Second)
		})

		Context("native ssl ", func() {

			BeforeEach(func() {
				// get the certificate so it is generated in the background
				go helpers.Certificate()
			})

			AfterEach(func() {
				err := kubeClient.CoreV1().Secrets(testHelper.InstallNamespace).Delete("secret", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("works with ssl", func() {
				createdSecret, err := kubeClient.CoreV1().Secrets(testHelper.InstallNamespace).Create(helpers.GetKubeSecret("secret", testHelper.InstallNamespace))
				Expect(err).NotTo(HaveOccurred())

				_, err = virtualServiceClient.Write(&v1.VirtualService{

					Metadata: core.Metadata{
						Name:      "vs",
						Namespace: testHelper.InstallNamespace,
					},
					SslConfig: &gloov1.SslConfig{
						SslSecrets: &gloov1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      createdSecret.ObjectMeta.Name,
								Namespace: createdSecret.ObjectMeta.Namespace,
							},
						},
					},
					VirtualHost: &gloov1.VirtualHost{
						Name:    "default",
						Domains: []string{"*"},
						Routes: []*gloov1.Route{{
							Matcher: &gloov1.Matcher{
								PathSpecifier: &gloov1.Matcher_Prefix{
									Prefix: "/",
								},
							},
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											Upstream: core.ResourceRef{
												Namespace: testHelper.InstallNamespace,
												Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, "testrunner", helper.TestRunnerPort)},
										},
									},
								},
							},
						}},
					},
				}, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
				// wait for default gateway to be created
				Eventually(func() (*v1.Gateway, error) {
					return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
				}, "15s", "0.5s").Should(Not(BeNil()))

				gatewayProxy := "gateway-proxy"
				gatewayPort := int(443)
				caFile := ToFile(helpers.Certificate())
				//noinspection GoUnhandledErrorResult
				defer os.Remove(caFile)

				err = setup.Kubectl("cp", caFile, testHelper.InstallNamespace+"/testrunner:/tmp/ca.crt")
				Expect(err).NotTo(HaveOccurred())

				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "https",
					Path:              "/",
					Method:            "GET",
					Host:              gatewayProxy,
					Service:           gatewayProxy,
					Port:              gatewayPort,
					CaFile:            "/tmp/ca.crt",
					ConnectionTimeout: 10,
				}, helper.SimpleHttpResponse, 1, 120*time.Second)
			})
		})
	})
	Context("upstream discovery", func() {
		var (
			createdServices []string
		)
		BeforeEach(func() {
			createdServices = nil
			//create some services
			for i := 0; i < 20; i++ {
				service := &corev1.Service{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:   fmt.Sprintf("testrunner-%d", i),
						Labels: map[string]string{"gloo": "testrunner"},
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"gloo": "testrunner"},
						Ports: []corev1.ServicePort{{
							Port: helper.TestRunnerPort,
						}},
					},
				}
				service, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Create(service)
				Expect(err).NotTo(HaveOccurred())
				createdServices = append(createdServices, service.Name)
			}
		})

		AfterEach(func() {
			for _, svcName := range createdServices {
				kubeClient.CoreV1().Services(testHelper.InstallNamespace).Delete(svcName, &meta_v1.DeleteOptions{})
			}
		})

		It("should preserve discovery", func() {
			getUpstream := func(svcname string) (*gloov1.Upstream, error) {
				upstreamName := fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, svcname, helper.TestRunnerPort)
				return upstreamClient.Read(testHelper.InstallNamespace, upstreamName, clients.ReadOpts{})
			}

			for _, svc := range createdServices {
				Eventually(func() (*gloov1.Upstream, error) { return getUpstream(svc) }, "15s", "0.5s").ShouldNot(BeNil())
				// now set subset config on an upstream:
				Eventually(func() error {
					upstream, _ := getUpstream(svc)
					upstream.UpstreamSpec.UpstreamType.(*gloov1.UpstreamSpec_Kube).Kube.ServiceSpec = &gloov1plugins.ServiceSpec{
						PluginType: &gloov1plugins.ServiceSpec_Grpc{
							Grpc: &grpcv1.ServiceSpec{},
						},
					}
					_, err := upstreamClient.Write(upstream, clients.WriteOpts{OverwriteExisting: true})
					return err
				}, "1s", "0.1s").ShouldNot(HaveOccurred())
			}

			// chill for a few letting discovery reconcile
			time.Sleep(time.Second * 10)

			// validate that all subset settings are still there
			for _, svc := range createdServices {
				// now set subset config on an upstream:
				up, _ := getUpstream(svc)
				spec := up.UpstreamSpec.UpstreamType.(*gloov1.UpstreamSpec_Kube).Kube.ServiceSpec
				Expect(spec).ToNot(BeNil())
				Expect(spec.GetGrpc()).ToNot(BeNil())
			}

		})

	})

	Context("with subsets and upstream groups", func() {

		var (
			redpod   *corev1.Pod
			bluepod  *corev1.Pod
			greenpod *corev1.Pod
			service  *corev1.Service
		)
		BeforeEach(func() {
			pod := &corev1.Pod{
				ObjectMeta: meta_v1.ObjectMeta{
					GenerateName: "pod",
					Labels:       map[string]string{"app": "redblue", "text": "red"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "echo",
						Image: "hashicorp/http-echo@sha256:ba27d460cd1f22a1a4331bdf74f4fccbc025552357e8a3249c40ae216275de96",
						Args:  []string{"-text=\"red-pod\""},
					}},
				}}
			var err error
			redpod, err = kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Create(pod)
			Expect(err).NotTo(HaveOccurred())

			pod.Labels["text"] = "blue"
			pod.Spec.Containers[0].Args = []string{"-text=\"blue-pod\""}
			bluepod, err = kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Create(pod)
			Expect(err).NotTo(HaveOccurred())

			// green pod - no label
			delete(pod.Labels, "text")
			pod.Spec.Containers[0].Args = []string{"-text=\"green-pod\""}
			greenpod, err = kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Create(pod)
			Expect(err).NotTo(HaveOccurred())

			service = &corev1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					GenerateName: "redblue",
					Labels:       map[string]string{"app": "redblue"},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "redblue"},
					Ports: []corev1.ServicePort{{
						Port: 5678,
					}},
				},
			}
			service, err = kubeClient.CoreV1().Services(testHelper.InstallNamespace).Create(service)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if redpod != nil {
				kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Delete(redpod.Name, &meta_v1.DeleteOptions{})
			}
			if bluepod != nil {
				kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Delete(bluepod.Name, &meta_v1.DeleteOptions{})
			}
			if greenpod != nil {
				kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Delete(greenpod.Name, &meta_v1.DeleteOptions{})
			}
			if service != nil {
				kubeClient.CoreV1().Services(testHelper.InstallNamespace).Delete(service.Name, &meta_v1.DeleteOptions{})
			}
		})

		It("routes to subsets and upstream groups", func() {

			getUpstream := func() (*gloov1.Upstream, error) {
				name := testHelper.InstallNamespace + "-" + service.Name + "-5678"
				return upstreamClient.Read(testHelper.InstallNamespace, name, clients.ReadOpts{})
			}
			// wait for upstream to be created
			Eventually(getUpstream, "15s", "0.5s").ShouldNot(BeNil())

			var upstreamRef core.ResourceRef
			// upstream write might error on a conflict so try it a few times
			// I use eventually so it will wait a bit between retries.
			Eventually(func() error {
				upstream, _ := getUpstream()
				upstream.UpstreamSpec.UpstreamType.(*gloov1.UpstreamSpec_Kube).Kube.SubsetSpec = &gloov1plugins.SubsetSpec{
					Selectors: []*gloov1plugins.Selector{{
						Keys: []string{"text"},
					}},
				}
				upstreamRef = upstream.Metadata.Ref()
				_, err := upstreamClient.Write(upstream, clients.WriteOpts{OverwriteExisting: true})
				return err
			}, "1s", "0.1s").ShouldNot(HaveOccurred())

			// add subsets to upstream
			ug := &gloov1.UpstreamGroup{
				Metadata: core.Metadata{
					Name:      "test",
					Namespace: testHelper.InstallNamespace,
				},
				Destinations: []*gloov1.WeightedDestination{
					{
						Weight: 1,
						Destination: &gloov1.Destination{
							Upstream: upstreamRef,
							Subset: &gloov1.Subset{
								Values: map[string]string{"text": "red"},
							},
						},
					},
					{
						Weight: 1,
						Destination: &gloov1.Destination{
							Upstream: upstreamRef,
							Subset: &gloov1.Subset{
								Values: map[string]string{"text": "blue"},
							},
						},
					},
					{
						Weight: 1,
						Destination: &gloov1.Destination{
							Upstream: upstreamRef,
							Subset: &gloov1.Subset{
								Values: map[string]string{"text": ""},
							},
						},
					},
				},
			}
			_, err := upstreamGroupClient.Write(ug, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			ugref := ug.Metadata.Ref()

			// create a pod
			// create an upstream group
			// add subset to the uptream
			// create another pod
			_, err = virtualServiceClient.Write(&v1.VirtualService{

				Metadata: core.Metadata{
					Name:      "vs",
					Namespace: testHelper.InstallNamespace,
				},
				VirtualHost: &gloov1.VirtualHost{
					Name:    "default",
					Domains: []string{"*"},
					Routes: []*gloov1.Route{
						{
							Matcher: &gloov1.Matcher{
								PathSpecifier: &gloov1.Matcher_Prefix{
									Prefix: "/red",
								},
							},
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											Upstream: upstreamRef,
											Subset: &gloov1.Subset{
												Values: map[string]string{"text": "red"},
											},
										},
									},
								},
							},
						}, {
							Matcher: &gloov1.Matcher{
								PathSpecifier: &gloov1.Matcher_Prefix{
									Prefix: "/",
								},
							},
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_UpstreamGroup{
										UpstreamGroup: &ugref,
									},
								},
							},
						},
					},
				},
			}, clients.WriteOpts{})

			Expect(err).NotTo(HaveOccurred())

			defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
			// wait for default gateway to be created
			Eventually(func() (*v1.Gateway, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			}, "15s", "0.5s").Should(Not(BeNil()))

			gatewayProxy := "gateway-proxy"
			gatewayPort := int(80)
			caFile := ToFile(helpers.Certificate())
			//noinspection GoUnhandledErrorResult
			defer os.Remove(caFile)

			// make sure we get both upstreams:
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 10,
			}, "red-pod", 1, 120*time.Second)

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 10,
			}, "blue-pod", 1, 120*time.Second)

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 10,
			}, "green-pod", 1, 120*time.Second)

			redOpts := helper.CurlOpts{
				Protocol:          "http",
				Path:              "/red",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 10,
			}

			// try it 10 times
			for i := 0; i < 10; i++ {
				res, err := testHelper.Curl(redOpts)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(ContainSubstring("red-pod"))
			}

		})
	})
})

func ToFile(content string) string {
	f, err := ioutil.TempFile("", "")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	n, err := f.WriteString(content)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, n).To(Equal(len(content)))
	_ = f.Close()
	return f.Name()
}
