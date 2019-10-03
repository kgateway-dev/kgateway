package gateway_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/solo-io/gloo/pkg/cliutil/install"

	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/testutils/exec"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"

	defaults2 "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	kubecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"

	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/go-utils/errors"

	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/linkerd"
	"github.com/solo-io/go-utils/testutils/helper"

	"k8s.io/client-go/kubernetes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

var _ = Describe("Kube2e: gateway", func() {

	const (
		gatewayProxy = translator.GatewayProxyName
		gatewayPort  = int(80)
	)

	var (
		ctx        context.Context
		cancel     context.CancelFunc
		cfg        *rest.Config
		cache      kube.SharedCache
		kubeClient kubernetes.Interface

		gatewayClient        gatewayv2.GatewayClient
		virtualServiceClient gatewayv1.VirtualServiceClient
		routeTableClient     gatewayv1.RouteTableClient
		upstreamGroupClient  gloov1.UpstreamGroupClient
		upstreamClient       gloov1.UpstreamClient
		proxyClient          gloov1.ProxyClient
		serviceClient        skkube.ServiceClient
	)

	var _ = BeforeEach(StartTestHelper)

	var _ = AfterEach(TearDownTestHelper)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		kubeClient, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		cache = kube.NewKubeCache(ctx)
		gatewayClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv2.GatewayCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		virtualServiceClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.VirtualServiceCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		routeTableClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.RouteTableCrd,
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
		proxyClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.ProxyCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}

		gatewayClient, err = gatewayv2.NewGatewayClient(gatewayClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = gatewayClient.Register()
		Expect(err).NotTo(HaveOccurred())

		virtualServiceClient, err = gatewayv1.NewVirtualServiceClient(virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualServiceClient.Register()
		Expect(err).NotTo(HaveOccurred())

		routeTableClient, err = gatewayv1.NewRouteTableClient(routeTableClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = routeTableClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamGroupClient, err = gloov1.NewUpstreamGroupClient(upstreamGroupClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamGroupClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamClient, err = gloov1.NewUpstreamClient(upstreamClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamClient.Register()
		Expect(err).NotTo(HaveOccurred())

		proxyClient, err = gloov1.NewProxyClient(proxyClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = proxyClient.Register()
		Expect(err).NotTo(HaveOccurred())

		kubeCoreCache, err := kubecache.NewKubeCoreCache(ctx, kubeClient)
		Expect(err).NotTo(HaveOccurred())
		serviceClient = service.NewServiceClient(kubeClient, kubeCoreCache)
	})

	It("removes all pods when uninstalled", func() {
		kubeInterface := kube2e.MustKubeClient().CoreV1()
		installedPods, err := kubeInterface.Pods(testHelper.InstallNamespace).List(metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "Should be able to read pods in the namespace")
		Expect(installedPods.Items).NotTo(BeEmpty(), "Should have a nonzero number of pods in the namespace")

		cmdArgs := []string{
			filepath.Join(testHelper.BuildAssetDir, testHelper.GlooctlExecName), "uninstall", "-n", testHelper.InstallNamespace,
		}

		err = exec.RunCommand(testHelper.RootDir, true, cmdArgs...)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be clean")

		Eventually(func() ([]corev1.Pod, error) {
			pods, err := kubeInterface.Pods(testHelper.InstallNamespace).List(metav1.ListOptions{})
			if err != nil {
				return nil, err
			}

			var runningPods []corev1.Pod
			for _, pod := range pods.Items {
				// the test runner itself is a pod in the namespace- we don't expect that one to be deleted
				if pod.ObjectMeta.Name != "testrunner" {
					runningPods = append(runningPods, pod)
				}
			}
			return runningPods, nil
		}, time.Minute, time.Second).Should(BeEmpty(), "There should be no pods remaining after running glooctl uninstall")
	})

	Context("tests with virtual service", func() {

		AfterEach(func() {
			cancel()
			err := virtualServiceClient.Delete(testHelper.InstallNamespace, "vs", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("correctly routes requests to an upstream", func() {
			dest := &gloov1.Destination{
				DestinationType: &gloov1.Destination_Upstream{
					Upstream: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
					},
				},
			}
			// give proxy validation a chance to start
			Eventually(func() error {
				_, err := virtualServiceClient.Write(getVirtualService(dest, nil), clients.WriteOpts{})
				return err
			}).ShouldNot(HaveOccurred())

			defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
			// wait for default gateway to be created
			Eventually(func() (*gatewayv2.Gateway, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			}, "15s", "0.5s").Should(Not(BeNil()))

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1, // this is important, as sometimes curl hangs
				WithoutStats:      true,
			}, helper.SimpleHttpResponse, 1, 60*time.Second, 1*time.Second)
		})

		Context("routing directly to kubernetes services", func() {

			BeforeEach(func() {

				// Create virtual service routing directly to the testrunner service
				dest := &gloov1.Destination{
					DestinationType: &gloov1.Destination_Kube{
						Kube: &gloov1.KubernetesServiceDestination{
							Ref: core.ResourceRef{
								Namespace: testHelper.InstallNamespace,
								Name:      helper.TestrunnerName,
							},
							Port: uint32(helper.TestRunnerPort),
						},
					},
				}
				_, err := virtualServiceClient.Write(getVirtualService(dest, nil), clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
			})

			It("correctly routes to the service (http)", func() {
				defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)

				// wait for default gateway to be created
				Eventually(func() *gatewayv2.Gateway {
					gw, _ := gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{Ctx: ctx})
					return gw
				}, "15s", "0.5s").Should(Not(BeNil()))

				// wait for the expected proxy configuration to be accepted
				Eventually(func() error {
					proxy, err := proxyClient.Read(testHelper.InstallNamespace, translator.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
					if err != nil {
						return err
					}

					if status := proxy.Status; status.State != core.Status_Accepted {
						return errors.Errorf("unexpected proxy state: %v. Reason: %v", status.State, status.Reason)
					}

					for _, l := range proxy.Listeners {
						for _, vh := range l.GetHttpListener().VirtualHosts {
							for _, r := range vh.Routes {
								if action := r.GetRouteAction(); action != nil {
									if single := action.GetSingle(); single != nil {
										if svcDest := single.GetKube(); svcDest != nil {
											if svcDest.Ref.Name == helper.TestrunnerName &&
												svcDest.Ref.Namespace == testHelper.InstallNamespace &&
												svcDest.Port == uint32(helper.TestRunnerPort) {
												return nil
											}
										}
									}
								}
							}
						}
					}

					return errors.Errorf("proxy did not contain expected route")
				}, "15s", "0.5s").Should(BeNil())

				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/",
					Method:            "GET",
					Host:              gatewayProxy,
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1, // this is important, as sometimes curl hangs
					WithoutStats:      true,
				}, helper.SimpleHttpResponse, 1, 60*time.Second, 1*time.Second)
			})

		})

		Context("native ssl", func() {

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

				dest := &gloov1.Destination{
					DestinationType: &gloov1.Destination_Upstream{
						Upstream: &core.ResourceRef{
							Namespace: testHelper.InstallNamespace,
							Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
						},
					},
				}

				sslConfig := &gloov1.SslConfig{
					SslSecrets: &gloov1.SslConfig_SecretRef{
						SecretRef: &core.ResourceRef{
							Name:      createdSecret.ObjectMeta.Name,
							Namespace: createdSecret.ObjectMeta.Namespace,
						},
					},
				}
				vs := getVirtualService(dest, sslConfig)

				// give Gloo a chance to pick up the secret
				// required to allow validation to pass
				Eventually(func() error {
					_, err = virtualServiceClient.Write(vs, clients.WriteOpts{})
					return err
				}, time.Second*5, time.Second).ShouldNot(HaveOccurred())
				Expect(err).NotTo(HaveOccurred())

				defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
				// wait for default gateway to be created
				Eventually(func() (*gatewayv2.Gateway, error) {
					return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
				}, "15s", "0.5s").Should(Not(BeNil()))

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
					Host:              translator.GatewayProxyName,
					Service:           translator.GatewayProxyName,
					Port:              gatewayPort,
					CaFile:            "/tmp/ca.crt",
					ConnectionTimeout: 1,
					WithoutStats:      true,
				}, helper.SimpleHttpResponse, 1, 60*time.Second, 1*time.Second)
			})
		})

		Context("linkerd enabled updates routes with appended headers", func() {
			var (
				settingsClient gloov1.SettingsClient
				httpEcho       helper.TestRunner
			)

			BeforeEach(func() {
				var err error
				settingsClientFactory := &factory.KubeResourceClientFactory{
					Crd:         gloov1.SettingsCrd,
					Cfg:         cfg,
					SharedCache: cache,
				}

				settingsClient, err = gloov1.NewSettingsClient(settingsClientFactory)
				Expect(err).NotTo(HaveOccurred())
				err = settingsClient.Register()
				Expect(err).NotTo(HaveOccurred())

				settingsList, err := settingsClient.List(testHelper.InstallNamespace, clients.ListOpts{})
				Expect(err).NotTo(HaveOccurred())
				Expect(settingsList).To(HaveLen(1))
				settings := settingsList[0]
				settings.Linkerd = true
				_, err = settingsClient.Write(settings, clients.WriteOpts{
					OverwriteExisting: true,
				})
				Expect(err).NotTo(HaveOccurred())

				httpEcho, err = helper.NewEchoHttp(testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred())

				err = httpEcho.Deploy(2 * time.Minute)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				settingsList, err := settingsClient.List(testHelper.InstallNamespace, clients.ListOpts{})
				Expect(err).NotTo(HaveOccurred())
				Expect(settingsList).To(HaveLen(1))
				settings := settingsList[0]
				settings.Linkerd = false
				_, err = settingsClient.Write(settings, clients.WriteOpts{
					OverwriteExisting: true,
				})
				Expect(err).NotTo(HaveOccurred())

				err = httpEcho.Terminate()
				Expect(err).NotTo(HaveOccurred())

				// TODO: Terminate() should do this as part of its cleanup
				err = serviceClient.Delete(testHelper.InstallNamespace, helper.HttpEchoName, clients.DeleteOpts{})
				Expect(err).NotTo(HaveOccurred())
			})

			It("appends linkerd headers when linkerd is enabled", func() {
				var us *gloov1.Upstream
				//give discovery time to write the upstream
				Eventually(func() error {
					upstreams, err := upstreamClient.List(testHelper.InstallNamespace, clients.ListOpts{})
					if err != nil {
						return err
					}
					upstreamName := fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.HttpEchoName, helper.HttpEchoPort)
					us, err = upstreams.Find(testHelper.InstallNamespace, upstreamName)
					return err
				}, time.Second*10, time.Second).ShouldNot(HaveOccurred())

				dest := &gloov1.Destination{
					DestinationType: &gloov1.Destination_Upstream{
						Upstream: utils.ResourceRefPtr(us.Metadata.Ref()),
					},
				}

				_, err := virtualServiceClient.Write(getVirtualService(dest, nil), clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				responseString := fmt.Sprintf(`"%s":"%s.%s.svc.cluster.local:%v"`,
					linkerd.HeaderKey, helper.HttpEchoName, testHelper.InstallNamespace, helper.HttpEchoPort)
				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/",
					Method:            "GET",
					Host:              gatewayProxy,
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1,
					WithoutStats:      true,
				}, responseString, 1, 60*time.Second, 1*time.Second)
			})
		})
	})

	Context("tests with route tables", func() {

		AfterEach(func() {
			cancel()
			err := virtualServiceClient.Delete(testHelper.InstallNamespace, "vs", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			err = routeTableClient.Delete(testHelper.InstallNamespace, "rt1", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			err = routeTableClient.Delete(testHelper.InstallNamespace, "rt2", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("correctly routes requests to an upstream", func() {
			dest := &gloov1.Destination{
				DestinationType: &gloov1.Destination_Upstream{
					Upstream: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
					},
				},
			}

			rt2 := getRouteTable("rt2", getRouteWithDest(dest, "/root/rt1/rt2"))
			rt1 := getRouteTable("rt1", getRouteWithDelegate(rt2.Metadata.Name, "/root/rt1"))
			vs := getVirtualServiceWithRoute(addPrefixRewrite(getRouteWithDelegate(rt1.Metadata.Name, "/root"), "/"), nil)

			_, err := routeTableClient.Write(rt1, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
			_, err = routeTableClient.Write(rt2, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			_, err = virtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
			// wait for default gateway to be created
			Eventually(func() (*gatewayv2.Gateway, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			}, "15s", "0.5s").Should(Not(BeNil()))

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/root/rt1/rt2",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1, // this is important, as sometimes curl hangs
				WithoutStats:      true,
			}, helper.SimpleHttpResponse, 1, 60*time.Second, 1*time.Second)
		})
	})

	Context("upstream discovery", func() {
		var createdServices []string

		var createServicesForPod = func(displayName string, port int32) {
			createdServices = nil
			// create some services
			for i := 0; i < 20; i++ {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:   fmt.Sprintf("%s-%d", displayName, i),
						Labels: map[string]string{"gloo": displayName},
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"gloo": displayName},
						Ports: []corev1.ServicePort{{
							Port: port,
						}},
					},
				}
				service, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Create(service)
				Expect(err).NotTo(HaveOccurred())
				createdServices = append(createdServices, service.Name)
			}
		}

		AfterEach(func() {
			for _, svcName := range createdServices {
				_ = kubeClient.CoreV1().Services(testHelper.InstallNamespace).Delete(svcName, &metav1.DeleteOptions{})
			}
		})

		It("should preserve discovery", func() {
			createServicesForPod(helper.TestrunnerName, helper.TestRunnerPort)
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

	Context("tcp", func() {

		var (
			defaultGateway *gatewayv2.Gateway
			tcpEcho        helper.TestRunner

			tcpPort = corev1.ServicePort{
				Name:       "tcp-proxy",
				Port:       int32(defaults2.TcpPort),
				TargetPort: intstr.FromInt(int(defaults2.TcpPort)),
				Protocol:   "TCP",
			}
		)

		BeforeEach(func() {
			var err error

			tcpEcho, err = helper.NewEchoTcp(testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(tcpEcho.Deploy(time.Minute)).NotTo(HaveOccurred())
			defaultGateway = defaults.DefaultTcpGateway(testHelper.InstallNamespace)
			dest := &gloov1.Destination{
				DestinationType: &gloov1.Destination_Kube{
					Kube: &gloov1.KubernetesServiceDestination{
						Ref: core.ResourceRef{
							Namespace: testHelper.InstallNamespace,
							Name:      helper.TcpEchoName,
						},
						Port: uint32(helper.TcpEchoPort),
					},
				},
			}
			tcpGateway := defaultGateway.GetTcpGateway()
			Expect(tcpGateway).NotTo(BeNil())
			tcpGateway.Destinations = append(tcpGateway.Destinations, &gloov1.TcpHost{
				Name: "one",
				Destination: &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_Single{
						Single: dest,
					},
				},
			})
			_, err = gatewayClient.Write(defaultGateway, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
			gwSvc, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Get(gatewayProxy, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			found := false
			for _, v := range gwSvc.Spec.Ports {
				if v.Name == tcpPort.Name || v.Port == tcpPort.Port {
					found = true
					break
				}
			}
			if !found {
				gwSvc.Spec.Ports = append(gwSvc.Spec.Ports, tcpPort)
			}
			_, err = kubeClient.CoreV1().Services(testHelper.InstallNamespace).Update(gwSvc)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(gatewayClient.Delete(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.DeleteOpts{})).NotTo(HaveOccurred())
			gwSvc, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Get(gatewayProxy, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			ports := make([]corev1.ServicePort, 0, len(gwSvc.Spec.Ports))
			for _, v := range gwSvc.Spec.Ports {
				if v.Name != tcpPort.Name {
					ports = append(ports, v)
				}
			}
			gwSvc.Spec.Ports = ports
			_, err = kubeClient.CoreV1().Services(testHelper.InstallNamespace).Update(gwSvc)
			Expect(err).NotTo(HaveOccurred())
			Expect(tcpEcho.Terminate()).NotTo(HaveOccurred())
		})

		It("correctly routes to the service (tcp)", func() {

			// wait for default gateway to be created
			Eventually(func() *gatewayv2.Gateway {
				gw, _ := gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{Ctx: ctx})
				return gw
			}, "15s", "0.5s").Should(Not(BeNil()))

			// wait for the expected proxy configuration to be accepted
			Eventually(func() error {
				proxy, err := proxyClient.Read(testHelper.InstallNamespace, translator.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				if err != nil {
					return err
				}

				if status := proxy.Status; status.State != core.Status_Accepted {
					return errors.Errorf("unexpected proxy state: %v. Reason: %v", status.State, status.Reason)
				}

				for _, l := range proxy.Listeners {
					tcpListener := l.GetTcpListener()
					if tcpListener == nil {
						continue
					}
					for _, tcph := range tcpListener.TcpHosts {
						if action := tcph.GetDestination(); action != nil {
							if single := action.GetSingle(); single != nil {
								if svcDest := single.GetKube(); svcDest != nil {
									if svcDest.Ref.Name == helper.TcpEchoName &&
										svcDest.Ref.Namespace == testHelper.InstallNamespace &&
										svcDest.Port == uint32(helper.TcpEchoPort) {
										return nil
									}
								}
							}
						}
					}
				}

				return errors.Errorf("proxy did not contain expected route")
			}, "15s", "0.5s").Should(BeNil())

			responseString := fmt.Sprintf("Connected to %s",
				gatewayProxy)

			tcpEcho.CurlEventuallyShouldOutput(helper.CurlOpts{
				Protocol:          "telnet",
				Service:           gatewayProxy,
				Port:              int(defaultGateway.BindPort),
				ConnectionTimeout: 10,
				Verbose:           true,
			}, responseString, 1, 30*time.Second)
		})
	})

	Context("with subsets and upstream groups", func() {

		var (
			redPod   *corev1.Pod
			bluePod  *corev1.Pod
			greenPod *corev1.Pod
			service  *corev1.Service
			vs       *gatewayv1.VirtualService
			ug       *gloov1.UpstreamGroup
		)
		BeforeEach(func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "pod",
					Labels:       map[string]string{"app": "redblue", "text": "red"},
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: pointerToInt64(0),
					Containers: []corev1.Container{{
						Name:  "echo",
						Image: "hashicorp/http-echo@sha256:ba27d460cd1f22a1a4331bdf74f4fccbc025552357e8a3249c40ae216275de96",
						Args:  []string{"-text=\"red-pod\""},
					}},
				}}
			var err error
			redPod, err = kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Create(pod)
			Expect(err).NotTo(HaveOccurred())

			pod.Labels["text"] = "blue"
			pod.Spec.Containers[0].Args = []string{"-text=\"blue-pod\""}
			bluePod, err = kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Create(pod)
			Expect(err).NotTo(HaveOccurred())

			// green pod - no label
			delete(pod.Labels, "text")
			pod.Spec.Containers[0].Args = []string{"-text=\"green-pod\""}
			greenPod, err = kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Create(pod)
			Expect(err).NotTo(HaveOccurred())

			service = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
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
			if redPod != nil {
				err := kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Delete(redPod.Name, &metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}
			if bluePod != nil {
				err := kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Delete(bluePod.Name, &metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}
			if greenPod != nil {
				err := kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Delete(greenPod.Name, &metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}
			if service != nil {
				err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Delete(service.Name, &metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}

			if vs != nil {
				err := virtualServiceClient.Delete(testHelper.InstallNamespace, vs.Metadata.Name, clients.DeleteOpts{IgnoreNotExist: true})
				Expect(err).NotTo(HaveOccurred())
			}

			if ug != nil {
				err := upstreamGroupClient.Delete(testHelper.InstallNamespace, ug.Metadata.Name, clients.DeleteOpts{IgnoreNotExist: true})
				Expect(err).NotTo(HaveOccurred())
			}

			Eventually(func() error {
				coloredPods, err := kubeClient.CoreV1().Pods(testHelper.InstallNamespace).List(
					metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": "redblue"}).String()})
				if err != nil {
					return err
				}
				vsList, err := virtualServiceClient.List(vs.Metadata.Namespace, clients.ListOpts{Ctx: ctx})
				if err != nil {
					return err
				}
				// After we remove the virtual service, the proxy should be removed as well by the gateway controller
				proxyList, err := proxyClient.List(testHelper.InstallNamespace, clients.ListOpts{Ctx: ctx})
				if err != nil {
					return err
				}

				if len(coloredPods.Items)+len(vsList)+len(proxyList) == 0 {
					return nil
				}
				return errors.Errorf("expected all test resources to have been deleted but found: "+
					"%d pods, %d virtual services, %d proxies", len(coloredPods.Items), len(vsList), len(proxyList))
			}, time.Minute, time.Second).Should(BeNil())
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
			ug = &gloov1.UpstreamGroup{
				Metadata: core.Metadata{
					Name:      "test",
					Namespace: testHelper.InstallNamespace,
				},
				Destinations: []*gloov1.WeightedDestination{
					{
						Weight: 1,
						Destination: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: &upstreamRef,
							},
							Subset: &gloov1.Subset{
								Values: map[string]string{"text": "red"},
							},
						},
					},
					{
						Weight: 1,
						Destination: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: &upstreamRef,
							},
							Subset: &gloov1.Subset{
								Values: map[string]string{"text": "blue"},
							},
						},
					},
					{
						Weight: 1,
						Destination: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: &upstreamRef,
							},
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
			// add subset to the upstream
			// create another pod
			vs, err = virtualServiceClient.Write(&gatewayv1.VirtualService{
				Metadata: core.Metadata{
					Name:      "vs",
					Namespace: testHelper.InstallNamespace,
				},
				VirtualHost: &gatewayv1.VirtualHost{
					Domains: []string{"*"},
					Routes: []*gatewayv1.Route{
						{
							Matcher: &gloov1.Matcher{
								PathSpecifier: &gloov1.Matcher_Prefix{
									Prefix: "/red",
								},
							},
							Action: &gatewayv1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: &upstreamRef,
											},
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
							Action: &gatewayv1.Route_RouteAction{
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
			Eventually(func() (*gatewayv2.Gateway, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			}, "15s", "0.5s").Should(Not(BeNil()))

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
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, "red-pod", 1, 120*time.Second, 1*time.Second)

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, "blue-pod", 1, 120*time.Second, 1*time.Second)

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, "green-pod", 1, 120*time.Second, 1*time.Second)

			redOpts := helper.CurlOpts{
				Protocol:          "http",
				Path:              "/red",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}

			// try it 10 times
			for i := 0; i < 10; i++ {
				res, err := testHelper.Curl(redOpts)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(ContainSubstring("red-pod"))
			}
		})
	})

	Context("tests for the validation server", func() {
		testValidation := func(yam, expectedErr string) {
			out, err := install.KubectlApplyOut([]byte(yam))
			if expectedErr == "" {
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				return
			}
			ExpectWithOffset(1, err).To(HaveOccurred())
			ExpectWithOffset(1, string(out)).To(ContainSubstring(expectedErr))
		}

		It("rejects bad resources", func() {
			// specifically avoiding using a DescribeTable here in order to avoid reinstalling
			// for every test case
			type testCase struct {
				resourceYaml, expectedErr string
			}

			for _, tc := range []testCase{
				{
					resourceYaml: `
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: ` + testHelper.InstallNamespace + `
spec:
  virtualHoost: {}
`,
					expectedErr: `could not unmarshal raw object: parsing resource from crd spec default in namespace ` + testHelper.InstallNamespace + ` into *v1.VirtualService: unknown field "virtualHoost" in v1.VirtualService`,
				},
				{
					resourceYaml: `
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: missing-upstream
  namespace: ` + testHelper.InstallNamespace + `
spec:
  virtualHost:
    routes:
      - matcher:
          methods:
            - GET
          prefix: /items/
        routeAction:
          single:
            upstream:
              name: does-not-exist
              namespace: anywhere
`,
					expectedErr: "", // should not fail
				},
				{
					resourceYaml: `
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: method-matcher
  namespace: ` + testHelper.InstallNamespace + `
spec:
  virtualHost:
    routes:
      - matcher:
          methods:
            - GET # not allowed
          prefix: /delegated-prefix
        delegateAction:
          name: does-not-exist # also not allowed, but caught later
          namespace: anywhere
`,
					expectedErr: "routes with delegate actions cannot use method matchers", // should not fail
				},
			} {
				testValidation(tc.resourceYaml, tc.expectedErr)
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

func getVirtualService(dest *gloov1.Destination, sslConfig *gloov1.SslConfig) *gatewayv1.VirtualService {
	return getVirtualServiceWithRoute(getRouteWithDest(dest, "/"), sslConfig)
}

func getVirtualServiceWithRoute(route *gatewayv1.Route, sslConfig *gloov1.SslConfig) *gatewayv1.VirtualService {
	return &gatewayv1.VirtualService{
		Metadata: core.Metadata{
			Name:      "vs",
			Namespace: testHelper.InstallNamespace,
		},
		SslConfig: sslConfig,
		VirtualHost: &gatewayv1.VirtualHost{
			Domains: []string{"*"},

			Routes: []*gatewayv1.Route{route},
		},
	}
}

func getRouteTable(name string, route *gatewayv1.Route) *gatewayv1.RouteTable {
	return &gatewayv1.RouteTable{
		Metadata: core.Metadata{
			Name:      name,
			Namespace: testHelper.InstallNamespace,
		},
		Routes: []*gatewayv1.Route{route},
	}
}

func getRouteWithDest(dest *gloov1.Destination, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		Matcher: &gloov1.Matcher{
			PathSpecifier: &gloov1.Matcher_Prefix{
				Prefix: path,
			},
		},
		Action: &gatewayv1.Route_RouteAction{
			RouteAction: &gloov1.RouteAction{
				Destination: &gloov1.RouteAction_Single{
					Single: dest,
				},
			},
		},
	}
}

func getRouteWithDelegate(delegate string, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		Matcher: &gloov1.Matcher{
			PathSpecifier: &gloov1.Matcher_Prefix{
				Prefix: path,
			},
		},
		Action: &gatewayv1.Route_DelegateAction{
			DelegateAction: &core.ResourceRef{
				Namespace: testHelper.InstallNamespace,
				Name:      delegate,
			},
		},
	}
}

func addPrefixRewrite(route *gatewayv1.Route, rewrite string) *gatewayv1.Route {
	route.RoutePlugins = &gloov1.RoutePlugins{PrefixRewrite: &transformation.PrefixRewrite{PrefixRewrite: rewrite}}
	return route
}
