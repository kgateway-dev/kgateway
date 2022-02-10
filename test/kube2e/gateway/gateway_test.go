package gateway_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/solo-io/solo-kit/test/setup"

	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/go-utils/testutils"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gwtranslator "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	clienthelpers "github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	grpcv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	gloorest "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/rest"
	glootransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	defaults2 "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	kubernetes2 "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/linkerd"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	kubecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	. "github.com/solo-io/solo-kit/test/matchers"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Kube2e: gateway", func() {

	const (
		gatewayProxy = defaults.GatewayProxyName
		gatewayPort  = int(80)
	)

	var (
		ctx        context.Context
		cancel     context.CancelFunc
		cfg        *rest.Config
		cache      kube.SharedCache
		kubeClient kubernetes.Interface

		gatewayClient           gatewayv1.GatewayClient
		httpGatewayClient           gatewayv1.MatchableHttpGatewayClient
		virtualServiceClient    gatewayv1.VirtualServiceClient
		routeTableClient        gatewayv1.RouteTableClient
		virtualHostOptionClient gatewayv1.VirtualHostOptionClient
		routeOptionClient       gatewayv1.RouteOptionClient
		upstreamGroupClient     gloov1.UpstreamGroupClient
		upstreamClient          gloov1.UpstreamClient
		proxyClient             gloov1.ProxyClient
		serviceClient           skkube.ServiceClient
		statusClient            resources.StatusClient
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		kubeClient, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		cache = kube.NewKubeCache(ctx)
		gatewayClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.GatewayCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		httpGatewayClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.MatchableHttpGatewayCrd,
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
		virtualHostOptionClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.VirtualHostOptionCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		routeOptionClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.RouteOptionCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}

		gatewayClient, err = gatewayv1.NewGatewayClient(ctx, gatewayClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = gatewayClient.Register()
		Expect(err).NotTo(HaveOccurred())

		httpGatewayClient, err = gatewayv1.NewMatchableHttpGatewayClient(ctx, httpGatewayClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = httpGatewayClient.Register()
		Expect(err).NotTo(HaveOccurred())

		virtualServiceClient, err = gatewayv1.NewVirtualServiceClient(ctx, virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualServiceClient.Register()
		Expect(err).NotTo(HaveOccurred())

		routeTableClient, err = gatewayv1.NewRouteTableClient(ctx, routeTableClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = routeTableClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamGroupClient, err = gloov1.NewUpstreamGroupClient(ctx, upstreamGroupClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamGroupClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamClient, err = gloov1.NewUpstreamClient(ctx, upstreamClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamClient.Register()
		Expect(err).NotTo(HaveOccurred())

		proxyClient, err = gloov1.NewProxyClient(ctx, proxyClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = proxyClient.Register()
		Expect(err).NotTo(HaveOccurred())

		virtualHostOptionClient, err = gatewayv1.NewVirtualHostOptionClient(ctx, virtualHostOptionClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualHostOptionClient.Register()
		Expect(err).NotTo(HaveOccurred())

		routeOptionClient, err = gatewayv1.NewRouteOptionClient(ctx, routeOptionClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = routeOptionClient.Register()
		Expect(err).NotTo(HaveOccurred())

		kubeCoreCache, err := kubecache.NewKubeCoreCache(ctx, kubeClient)
		Expect(err).NotTo(HaveOccurred())
		serviceClient = service.NewServiceClient(kubeClient, kubeCoreCache)

		statusClient = gloostatusutils.GetStatusClientForNamespace(testHelper.InstallNamespace)
	})

	AfterEach(func() {
		cancel()
	})

	Context("tests with orphaned gateways", func() {
		It("correctly sets a status to a single gateway", func() {
			defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
			// wait for default gateway to be created
			Eventually(func() (*gatewayv1.Gateway, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			}, "15s", "0.5s").Should(Not(BeNil()))

			// demand that a created gateway _has_ a status.  This test is "good enough", as, prior to an orphaned gateway fix,
			// https://github.com/solo-io/gloo/pull/5790, free-floating gateways would never be assigned a status at all (nil)
			gw, _ := gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			Expect(gw.NamespacedStatuses.GetStatuses()).NotTo(BeNil())
		})
	})

	Context("tests with virtual service", func() {

		AfterEach(func() {
			err := virtualServiceClient.Delete(testHelper.InstallNamespace, "vs", clients.DeleteOpts{IgnoreNotExist: true})
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable("can route to upstream", func(compressedProxy bool) {

			kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
				Expect(settings.GetGateway().GetCompressedProxySpec()).NotTo(BeNil())
				settings.GetGateway().CompressedProxySpec = compressedProxy
			}, testHelper.InstallNamespace)

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
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			})

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1, // this is important, as sometimes curl hangs
				WithoutStats:      true,
			}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)

			kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
				Expect(settings.GetGateway().GetCompressedProxySpec()).NotTo(BeNil())
				settings.GetGateway().CompressedProxySpec = false
			}, testHelper.InstallNamespace)
		},
			Entry("can route to upstreams", false),
			Entry("can route to upstreams with compressed proxy", true))

		Context("routing directly to kubernetes services", func() {

			BeforeEach(func() {

				// Create virtual service routing directly to the testrunner service
				dest := &gloov1.Destination{
					DestinationType: &gloov1.Destination_Kube{
						Kube: &gloov1.KubernetesServiceDestination{
							Ref: &core.ResourceRef{
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
				Eventually(func() *gatewayv1.Gateway {
					gw, _ := gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{Ctx: ctx})
					return gw
				}, "15s", "0.5s").Should(Not(BeNil()))

				// wait for the expected proxy configuration to be accepted
				Eventually(func() error {
					proxy, err := proxyClient.Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
					if err != nil {
						return err
					}

					proxyStatus := statusClient.GetStatus(proxy)
					if proxyStatus.GetState() != core.Status_Accepted {
						return eris.Errorf("unexpected proxy state: %v. Reason: %v", proxyStatus, proxyStatus.GetReason())
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

					return eris.Errorf("proxy did not contain expected route")
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
				}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)
			})

		})

		Context("native ssl", func() {

			BeforeEach(func() {
				// get the certificate so it is generated in the background
				go helpers.Certificate()
			})

			AfterEach(func() {
				err := kubeClient.CoreV1().Secrets(testHelper.InstallNamespace).Delete(ctx, "secret", metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
			})

			It("works with ssl", func() {
				createdSecret, err := kubeClient.CoreV1().Secrets(testHelper.InstallNamespace).Create(ctx, helpers.GetKubeSecret("secret", testHelper.InstallNamespace), metav1.CreateOptions{})
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
				}, time.Second*10, time.Second).ShouldNot(HaveOccurred())
				Expect(err).NotTo(HaveOccurred())

				defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
				// wait for default gateway to be created
				Eventually(func() (*gatewayv1.Gateway, error) {
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
					Host:              defaults.GatewayProxyName,
					Service:           defaults.GatewayProxyName,
					Port:              gatewayPort,
					CaFile:            "/tmp/ca.crt",
					ConnectionTimeout: 1,
					WithoutStats:      true,
				}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)
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
					SharedCache: kube.NewKubeCache(ctx),
				}

				settingsClient, err = gloov1.NewSettingsClient(ctx, settingsClientFactory)
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
				upstreamName := fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.HttpEchoName, helper.HttpEchoPort)
				var ref *core.ResourceRef
				// give discovery time to write the upstream
				Eventually(func() error {
					upstreams, err := upstreamClient.List(testHelper.InstallNamespace, clients.ListOpts{})
					if err != nil {
						return err
					}
					us, err := upstreams.Find(testHelper.InstallNamespace, upstreamName)
					if err != nil {
						return err
					}
					ref = us.Metadata.Ref()
					return nil
				}, time.Second*10, time.Second).ShouldNot(HaveOccurred())

				dest := &gloov1.Destination{
					DestinationType: &gloov1.Destination_Upstream{
						Upstream: ref,
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

		Context("with a mix of valid and invalid virtual services", func() {

			var (
				validVsName   = "i-am-valid"
				invalidVsName = "i-am-invalid"
				petstoreName  = "petstore"
			)

			BeforeEach(func() {
				valid := withName(validVsName, withDomains([]string{"valid.com"},
					getVirtualService(&gloov1.Destination{
						DestinationType: &gloov1.Destination_Upstream{
							Upstream: &core.ResourceRef{
								Namespace: testHelper.InstallNamespace,
								Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
							},
						},
					}, nil)))
				inValid := withName(invalidVsName, withDomains([]string{"invalid.com"},
					getVirtualServiceWithRoute(&gatewayv1.Route{
						Matchers: []*matchers.Matcher{{}},
						Options: &gloov1.RouteOptions{
							PrefixRewrite: &wrappers.StringValue{Value: "matcher and action are missing"},
						},
					}, nil)))

				_, err := virtualServiceClient.Write(valid, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				// sanity check that validation is enabled/strict
				_, err = virtualServiceClient.Write(inValid, clients.WriteOpts{})
				Expect(err).To(MatchError(ContainSubstring("could not render proxy")))

				// disable strict validation
				kube2e.UpdateAlwaysAcceptSetting(ctx, true, testHelper.InstallNamespace)

				// eventually we are able to write the invalid vs
				Eventually(func() error {
					_, err := virtualServiceClient.Write(inValid, clients.WriteOpts{})
					return err
				}, time.Second*10).ShouldNot(HaveOccurred())

			})

			AfterEach(func() {
				_ = virtualServiceClient.Delete(testHelper.InstallNamespace, invalidVsName, clients.DeleteOpts{})
				helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
					return virtualServiceClient.Read(testHelper.InstallNamespace, invalidVsName, clients.ReadOpts{})
				}, "15s", "0.5s")

				_ = virtualServiceClient.Delete(testHelper.InstallNamespace, validVsName, clients.DeleteOpts{})
				helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
					return virtualServiceClient.Read(testHelper.InstallNamespace, validVsName, clients.ReadOpts{})
				}, "15s", "0.5s")

				_ = virtualServiceClient.Delete(testHelper.InstallNamespace, petstoreName, clients.DeleteOpts{})
				helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
					return virtualServiceClient.Read(testHelper.InstallNamespace, petstoreName, clients.ReadOpts{})
				}, "15s", "0.5s")

				// important that we update the always accept setting after removing resources, or else we can have:
				// "validation is disabled due to an invalid resource which has been written to storage.
				// Please correct any Rejected resources to re-enable validation."
				kube2e.UpdateAlwaysAcceptSetting(ctx, false, testHelper.InstallNamespace)
			})

			It("propagates the valid virtual services to envoy", func() {
				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/",
					Method:            "GET",
					Host:              "valid.com",
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1, // this is important, as sometimes curl hangs
					WithoutStats:      true,
				}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)
				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/",
					Method:            "GET",
					Host:              "invalid.com",
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1, // this is important, as sometimes curl hangs
					WithoutStats:      true,
					Verbose:           true,
				}, `HTTP/1.1 404 Not Found`, 1, 60*time.Second, 1*time.Second)
			})

			It("preserves the valid virtual services in envoy when a virtual service has been made invalid", func() {
				invalidVs, err := virtualServiceClient.Read(testHelper.InstallNamespace, invalidVsName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				validVs, err := virtualServiceClient.Read(testHelper.InstallNamespace, validVsName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// make the invalid vs valid and the valid vs invalid
				invalidVh := invalidVs.VirtualHost
				validVh := validVs.VirtualHost
				validVh.Domains = []string{"all-good-in-the-hood.com"}

				invalidVs.VirtualHost = validVh
				validVs.VirtualHost = invalidVh

				virtualServiceReconciler := gatewayv1.NewVirtualServiceReconciler(virtualServiceClient, statusClient)
				err = virtualServiceReconciler.Reconcile(testHelper.InstallNamespace, gatewayv1.VirtualServiceList{validVs, invalidVs}, nil, clients.ListOpts{})
				Expect(err).NotTo(HaveOccurred())

				// the original virtual service should work
				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/",
					Method:            "GET",
					Host:              "valid.com",
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1, // this is important, as sometimes curl hangs
					WithoutStats:      true,
				}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)

				// the fixed virtual service should also work
				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/",
					Method:            "GET",
					Host:              "all-good-in-the-hood.com",
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1, // this is important, as sometimes curl hangs
					WithoutStats:      true,
				}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)
			})

			Context("adds the invalid virtual services back into the proxy", func() {

				var (
					err                error
					petstoreSvc        *corev1.Service
					petstoreDeployment *v1.Deployment
				)

				BeforeEach(func() {
					petstoreDeployment, petstoreSvc = petstore(testHelper.InstallNamespace)

					// disable FDS for the petstore, create it without functions
					petstoreSvc.Labels[syncer.FdsLabelKey] = "disabled"

					petstoreSvc, err = kubeClient.CoreV1().Services(petstoreSvc.Namespace).Create(ctx, petstoreSvc, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())
					petstoreDeployment, err = kubeClient.AppsV1().Deployments(petstoreDeployment.Namespace).Create(ctx, petstoreDeployment, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					_ = virtualServiceClient.Delete(petstoreSvc.Namespace, petstoreName, clients.DeleteOpts{})
					helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
						return virtualServiceClient.Read(petstoreSvc.Namespace, petstoreName, clients.ReadOpts{})
					}, "15s", "0.5s")

					_ = kubeClient.CoreV1().Services(petstoreSvc.Namespace).Delete(ctx, petstoreName, metav1.DeleteOptions{})
					helpers.EventuallyObjectDeleted(func() (client.Object, error) {
						return kubeClient.CoreV1().Services(petstoreSvc.Namespace).Get(ctx, petstoreName, metav1.GetOptions{})
					}, "15s", "0.5s")

					_ = kubeClient.AppsV1().Deployments(petstoreDeployment.Namespace).Delete(ctx, petstoreName, metav1.DeleteOptions{})
					helpers.EventuallyObjectDeleted(func() (client.Object, error) {
						return kubeClient.AppsV1().Deployments(petstoreDeployment.Namespace).Get(ctx, petstoreName, metav1.GetOptions{})
					}, "15s", "0.5s")
				})

				It("when updating an upstream makes them valid", func() {
					upstreamName := fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, petstoreName, 8080)

					// the vs will be invalid
					vsWithFunctionRoute := withName(petstoreName, withDomains([]string{"petstore.com"},
						getVirtualService(&gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Namespace: testHelper.InstallNamespace,
									Name:      upstreamName,
								},
							},
							DestinationSpec: &gloov1.DestinationSpec{
								DestinationType: &gloov1.DestinationSpec_Rest{
									Rest: &gloorest.DestinationSpec{
										FunctionName: "findPetById",
									},
								},
							},
						}, nil)))

					vsWithFunctionRoute, err = virtualServiceClient.Write(vsWithFunctionRoute, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					// the VS should not be rejected since the failure is sanitized by route replacement
					helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
						return virtualServiceClient.Read(testHelper.InstallNamespace, petstoreName, clients.ReadOpts{})
					})

					// wrapped in eventually to get around resource version errors
					Eventually(func() error {
						petstoreUs, err := upstreamClient.Read(testHelper.InstallNamespace, upstreamName, clients.ReadOpts{})
						Expect(err).NotTo(HaveOccurred())

						Expect(petstoreUs.GetKube().GetServiceSpec().GetRest().GetSwaggerInfo().GetUrl()).To(BeEmpty())
						petstoreUs.Metadata.Labels[syncer.FdsLabelKey] = "enabled"

						_, err = upstreamClient.Write(petstoreUs, clients.WriteOpts{OverwriteExisting: true})
						return err
					}, "5s", "0.5s").ShouldNot(HaveOccurred())

					// FDS should update the upstream with discovered rest spec
					// it can take a long time for this to happen, perhaps petstore wasn't healthy yet?
					Eventually(func() interface{} {
						petstoreUs, err := upstreamClient.Read(testHelper.InstallNamespace, upstreamName, clients.ReadOpts{})
						Expect(err).ToNot(HaveOccurred())
						return petstoreUs.GetKube().GetServiceSpec().GetRest().GetSwaggerInfo().GetUrl()
					}, "120s", "1s").ShouldNot(BeEmpty())

					// we have updated an upstream, which prompts Gloo to send a notification to the
					// gateway to resync virtual service status

					// the VS should get accepted
					helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
						return virtualServiceClient.Read(vsWithFunctionRoute.GetMetadata().GetNamespace(), vsWithFunctionRoute.GetMetadata().GetName(), clients.ReadOpts{})
					})
				})

			})

		})

		Context("with a mix of valid and invalid routes on a single virtual service", func() {

			Context("route destination is nonexistent upstream", func() {
				var vs *gatewayv1.VirtualService

				BeforeEach(func() {
					kube2e.UpdateReplaceInvalidRoutes(ctx, true, testHelper.InstallNamespace)

					vs = withRoute(&gatewayv1.Route{
						Matchers: []*matchers.Matcher{{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/invalid-route"}}},
						Action: &gatewayv1.Route_RouteAction{RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{Single: &gloov1.Destination{
								DestinationType: &gloov1.Destination_Upstream{
									Upstream: &core.ResourceRef{
										Namespace: testHelper.InstallNamespace,
										Name:      "does-not-exist",
									},
								},
							}},
						}},
					}, getVirtualService(&gloov1.Destination{
						DestinationType: &gloov1.Destination_Upstream{
							Upstream: &core.ResourceRef{
								Namespace: testHelper.InstallNamespace,
								Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
							},
						},
					}, nil))

					_, err := virtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					_ = virtualServiceClient.Delete(vs.Metadata.Namespace, vs.Metadata.Name, clients.DeleteOpts{})
					helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
						return virtualServiceClient.Read(vs.GetMetadata().GetNamespace(), vs.GetMetadata().GetName(), clients.ReadOpts{})
					}, "15s", "0.5s")

					kube2e.UpdateReplaceInvalidRoutes(ctx, false, testHelper.InstallNamespace)
				})

				It("serves a direct response for the invalid route response", func() {
					// the valid route should work
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/",
						Method:            "GET",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1, // this is important, as sometimes curl hangs
						WithoutStats:      true,
					}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)

					// the invalid route should respond with the direct response
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/invalid-route",
						Method:            "GET",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1, // this is important, as sometimes curl hangs
						WithoutStats:      true,
					}, "Gloo Gateway has invalid configuration", 1, 60*time.Second, 1*time.Second)
				})
			})

			Context("route prefix is invalid", func() {
				var vs *gatewayv1.VirtualService
				var goodRt *gatewayv1.RouteTable
				var badRt *gatewayv1.RouteTable

				AfterEach(func() {
					// delete VS and 2 RTs
					err := virtualServiceClient.Delete(vs.GetMetadata().GetNamespace(), vs.GetMetadata().GetName(), clients.DeleteOpts{})
					Expect(err).NotTo(HaveOccurred())
					helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
						return virtualServiceClient.Read(vs.GetMetadata().GetNamespace(), vs.GetMetadata().GetName(), clients.ReadOpts{})
					}, "15s", "0.5s")

					err = routeTableClient.Delete(testHelper.InstallNamespace, "good-rt", clients.DeleteOpts{})
					Expect(err).NotTo(HaveOccurred())
					helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
						return routeTableClient.Read(goodRt.GetMetadata().GetNamespace(), goodRt.GetMetadata().GetName(), clients.ReadOpts{})
					}, "15s", "0.5s")

					err = routeTableClient.Delete(testHelper.InstallNamespace, "bad-rt", clients.DeleteOpts{})
					Expect(err).NotTo(HaveOccurred())
					helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
						return routeTableClient.Read(badRt.GetMetadata().GetNamespace(), badRt.GetMetadata().GetName(), clients.ReadOpts{})
					}, "15s", "0.5s")
				})

				It("invalid route delegated via ref does not prevent updates to valid routes", func() {
					// create a VS with 2 delegated RTs
					goodVsRoute := getRouteWithDelegateRef("good-rt", "/route-1")
					badVsRoute := getRouteWithDelegateRef("bad-rt", "/route-2")
					vs = getVirtualServiceWithRoute(goodVsRoute, nil)
					vs = withRoute(badVsRoute, vs)

					goodRt = getRouteTable("good-rt", nil, getRouteWithDirectResponse("Good response", "/route-1"))
					// bad RT's prefix does not start with parent's prefix, which should be a warning
					badRt = getRouteTable("bad-rt", nil, getRouteWithDirectResponse("Bad response", "/does-not-match"))

					// write the VS and RTs
					_, err := virtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())
					_, err = routeTableClient.Write(goodRt, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())
					_, err = routeTableClient.Write(badRt, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					// the good RT should be accepted, but both the VS and bad RT should have a warning
					helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
						return virtualServiceClient.Read(vs.GetMetadata().GetNamespace(), vs.GetMetadata().GetName(), clients.ReadOpts{})
					})
					helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
						return routeTableClient.Read(goodRt.GetMetadata().GetNamespace(), goodRt.GetMetadata().GetName(), clients.ReadOpts{})
					})
					helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
						return routeTableClient.Read(badRt.GetMetadata().GetNamespace(), badRt.GetMetadata().GetName(), clients.ReadOpts{})
					})

					// the valid route should return the expected direct response
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/route-1",
						Method:            "GET",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, "Good response", 1, 60*time.Second, 1*time.Second)

					// the invalid route should return a 404
					res, err := testHelper.Curl(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/route-2",
						Method:            "GET",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
						ReturnHeaders:     true,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(res).To(ContainSubstring("404 Not Found"))

					// update the response of the good RT
					goodRt, err = routeTableClient.Read(testHelper.InstallNamespace, "good-rt", clients.ReadOpts{})
					Expect(err).NotTo(HaveOccurred())
					goodRt.Routes[0] = getRouteWithDirectResponse("Updated good response", "/route-1")
					_, err = routeTableClient.Write(goodRt, clients.WriteOpts{OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())

					// make sure it returns the new response
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/route-1",
						Method:            "GET",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, "Updated good response", 1, 60*time.Second, 1*time.Second)
				})

				It("invalid route delegated via selector does not prevent updates to valid routes", func() {
					// create a VS that selects delegate RTs via labels
					labels := map[string]string{"select": "me"}
					vsRoute := getRouteWithDelegateSelector(labels, "/foo")
					vs = getVirtualServiceWithRoute(vsRoute, nil)

					goodRt = getRouteTable("good-rt", labels, getRouteWithDirectResponse("Good response", "/foo/a"))
					// bad RT's prefix does not start with parent's prefix, which should be a warning
					badRt = getRouteTable("bad-rt", labels, getRouteWithDirectResponse("Bad response", "/does-not-match"))

					// write the VS and RTs
					_, err := virtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())
					_, err = routeTableClient.Write(goodRt, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())
					_, err = routeTableClient.Write(badRt, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					// the good RT should be accepted, but both the VS and bad RT should have a warning
					helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
						return virtualServiceClient.Read(vs.GetMetadata().GetNamespace(), vs.GetMetadata().GetName(), clients.ReadOpts{})
					})
					helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
						return routeTableClient.Read(goodRt.GetMetadata().GetNamespace(), goodRt.GetMetadata().GetName(), clients.ReadOpts{})
					})
					helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
						return routeTableClient.Read(badRt.GetMetadata().GetNamespace(), badRt.GetMetadata().GetName(), clients.ReadOpts{})
					})

					// the valid route should return the expected direct response
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/foo/a",
						Method:            "GET",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, "Good response", 1, 60*time.Second, 1*time.Second)

					// update the response of the good RT
					goodRt, err = routeTableClient.Read(testHelper.InstallNamespace, "good-rt", clients.ReadOpts{})
					Expect(err).NotTo(HaveOccurred())
					goodRt.Routes[0] = getRouteWithDirectResponse("Updated good response", "/foo/a")
					_, err = routeTableClient.Write(goodRt, clients.WriteOpts{OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())

					// make sure it returns the new response
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/foo/a",
						Method:            "GET",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, "Updated good response", 1, 60*time.Second, 1*time.Second)
				})
			})
		})
	})

	Context("tests with route tables", func() {

		AfterEach(func() {
			err := virtualServiceClient.Delete(testHelper.InstallNamespace, "vs", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			err = routeTableClient.Delete(testHelper.InstallNamespace, "rt1", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			err = routeTableClient.Delete(testHelper.InstallNamespace, "rt2", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			cancel()
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

			rt2 := getRouteTable("rt2", nil, getRouteWithDest(dest, "/root/rt1/rt2"))
			rt1 := getRouteTable("rt1", nil, getRouteWithDelegateRef(rt2.Metadata.Name, "/root/rt1"))
			vs := getVirtualServiceWithRoute(addPrefixRewrite(getRouteWithDelegateRef(rt1.Metadata.Name, "/root"), "/"), nil)

			_, err := routeTableClient.Write(rt1, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
			_, err = routeTableClient.Write(rt2, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			_, err = virtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
			// wait for default gateway to be created
			Eventually(func() (*gatewayv1.Gateway, error) {
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
			}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)
		})
	})

	Context("tests with VirtualHostOptions", func() {

		AfterEach(func() {
			err := virtualServiceClient.Delete(testHelper.InstallNamespace, "vs", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			err = virtualHostOptionClient.Delete(testHelper.InstallNamespace, "vh-opt-one", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			err = virtualHostOptionClient.Delete(testHelper.InstallNamespace, "vh-opt-two", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			cancel()
		})

		It("correctly delegates options from VirtualHostOption", func() {
			dest := &gloov1.Destination{
				DestinationType: &gloov1.Destination_Upstream{
					Upstream: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
					},
				},
			}
			vh1 := &gatewayv1.VirtualHostOption{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "vh-opt-one",
				},
				Options: &gloov1.VirtualHostOptions{
					HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToRemove: []string{"header-from-external-options1"},
					},
					Cors: &cors.CorsPolicy{
						ExposeHeaders: []string{"header-from-extopt1"},
						AllowOrigin:   []string{"some-origin-1"},
					},
				},
			}
			vh2 := &gatewayv1.VirtualHostOption{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "vh-opt-two",
				},
				Options: &gloov1.VirtualHostOptions{
					HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToRemove: []string{"header-from-external-options2"},
					},
					Cors: &cors.CorsPolicy{
						ExposeHeaders: []string{"header-from-extopt2"},
						AllowOrigin:   []string{"some-origin-2"},
					},
					Transformations: &glootransformation.Transformations{
						RequestTransformation: &glootransformation.Transformation{
							TransformationType: &glootransformation.Transformation_TransformationTemplate{
								TransformationTemplate: &transformation.TransformationTemplate{
									Headers: map[string]*transformation.InjaTemplate{
										"x-header-added-in-opt2": {
											Text: "this header was added in the VirtualHostOption object vhOpt2",
										},
									},
								},
							},
						},
					},
				},
			}

			vs := &gatewayv1.VirtualService{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "vs",
				},
				VirtualHost: &gatewayv1.VirtualHost{
					Domains: []string{"*"},
					Routes:  []*gatewayv1.Route{getRouteWithDest(dest, "/")},
					Options: &gloov1.VirtualHostOptions{
						HeaderManipulation: &headers.HeaderManipulation{
							RequestHeadersToRemove: []string{"header-from-vhost"},
						},
					},
					ExternalOptionsConfig: &gatewayv1.VirtualHost_OptionsConfigRefs{
						OptionsConfigRefs: &gatewayv1.DelegateOptionsRefs{
							DelegateOptions: []*core.ResourceRef{
								{
									Namespace: testHelper.InstallNamespace,
									Name:      "vh-opt-one",
								},
								{
									Namespace: testHelper.InstallNamespace,
									Name:      "vh-opt-two",
								},
							},
						},
					},
				},
			}

			_, err := virtualHostOptionClient.Write(vh1, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
			_, err = virtualHostOptionClient.Write(vh2, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// give vhost options a chance to propogate
			Eventually(func() error {
				_, err := virtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
				return err
			}, "5s", "0.1s").ShouldNot(HaveOccurred())

			var proxy *gloov1.Proxy
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				proxy, err = proxyClient.Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				return proxy, err
			}, "15s", ".5s")

			var found bool
			for _, l := range proxy.Listeners {
				httpListener := l.GetHttpListener()
				if httpListener == nil {
					continue
				}
				for _, vhost := range httpListener.GetVirtualHosts() {
					found = true
					opts := vhost.GetOptions()
					// option config on VirtualHost overrides all delegated options
					testutils.ExpectEqualProtoMessages(opts.GetHeaderManipulation(), vs.GetVirtualHost().GetOptions().GetHeaderManipulation())
					// since rt1 is delegated to first, it overrides rt2, which was delegated later
					testutils.ExpectEqualProtoMessages(opts.GetCors(), vh1.GetOptions().GetCors())
					// options that weren't already set in previously delegated options are set from rt2
					testutils.ExpectEqualProtoMessages(opts.GetTransformations(), vh2.GetOptions().GetTransformations())
				}
			}
			Expect(found).To(BeTrue())
		})
	})

	Context("tests with RouteOptions", func() {

		AfterEach(func() {
			err := virtualServiceClient.Delete(testHelper.InstallNamespace, "vs", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			err = routeOptionClient.Delete(testHelper.InstallNamespace, "rt-opt-one", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			err = routeOptionClient.Delete(testHelper.InstallNamespace, "rt-opt-two", clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			cancel()
		})

		It("correctly delegates options from RouteOption", func() {
			dest := &gloov1.Destination{
				DestinationType: &gloov1.Destination_Upstream{
					Upstream: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
					},
				},
			}
			rt1 := &gatewayv1.RouteOption{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "rt-opt-one",
				},
				Options: &gloov1.RouteOptions{
					HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToRemove: []string{"header-from-external-options1"},
					},
					Cors: &cors.CorsPolicy{
						ExposeHeaders: []string{"header-from-extopt1"},
						AllowOrigin:   []string{"some-origin-1"},
					},
				},
			}
			rt2 := &gatewayv1.RouteOption{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "rt-opt-two",
				},
				Options: &gloov1.RouteOptions{
					HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToRemove: []string{"header-from-external-options2"},
					},
					Cors: &cors.CorsPolicy{
						ExposeHeaders: []string{"header-from-extopt2"},
						AllowOrigin:   []string{"some-origin-2"},
					},
					Transformations: &glootransformation.Transformations{
						RequestTransformation: &glootransformation.Transformation{
							TransformationType: &glootransformation.Transformation_TransformationTemplate{
								TransformationTemplate: &transformation.TransformationTemplate{
									Headers: map[string]*transformation.InjaTemplate{
										"x-header-added-in-opt2": {
											Text: "this header was added in the VirtualHostOption object vhOpt2",
										},
									},
								},
							},
						},
					},
				},
			}

			vs := &gatewayv1.VirtualService{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "vs",
				},
				VirtualHost: &gatewayv1.VirtualHost{
					Domains: []string{"*"},
					Routes: []*gatewayv1.Route{
						{
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/",
								},
							}},
							Action: &gatewayv1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: dest,
									},
								},
							},
							Options: &gloov1.RouteOptions{
								HeaderManipulation: &headers.HeaderManipulation{
									RequestHeadersToRemove: []string{"header-from-vhost"},
								},
							},
							ExternalOptionsConfig: &gatewayv1.Route_OptionsConfigRefs{
								OptionsConfigRefs: &gatewayv1.DelegateOptionsRefs{
									DelegateOptions: []*core.ResourceRef{
										{
											Namespace: testHelper.InstallNamespace,
											Name:      "rt-opt-one",
										},
										{
											Namespace: testHelper.InstallNamespace,
											Name:      "rt-opt-two",
										},
									},
								},
							},
						},
					},
				},
			}

			_, err := routeOptionClient.Write(rt1, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
			_, err = routeOptionClient.Write(rt2, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// give settings a chance to propogate
			Eventually(func() error {
				_, err := virtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
				return err
			}, "5s", "0.1s").ShouldNot(HaveOccurred())

			var proxy *gloov1.Proxy
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				proxy, err = proxyClient.Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				return proxy, err
			}, "15s", ".5s")

			var found bool
			for _, l := range proxy.Listeners {
				httpListener := l.GetHttpListener()
				if httpListener == nil {
					continue
				}
				for _, vhost := range httpListener.GetVirtualHosts() {
					for _, route := range vhost.GetRoutes() {
						found = true
						opts := route.GetOptions()
						// option config on VirtualHost overrides all delegated options
						testutils.ExpectEqualProtoMessages(opts.GetHeaderManipulation(), vs.GetVirtualHost().GetRoutes()[0].GetOptions().GetHeaderManipulation())
						// since rt1 is delegated to first, it overrides rt2, which was delegated later
						testutils.ExpectEqualProtoMessages(opts.GetCors(), rt1.GetOptions().GetCors())
						// options that weren't already set in previously delegated options are set from rt2
						testutils.ExpectEqualProtoMessages(opts.GetTransformations(), rt2.GetOptions().GetTransformations())
					}
				}
			}
			Expect(found).To(BeTrue())
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
				service, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Create(ctx, service, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				createdServices = append(createdServices, service.Name)
			}
		}

		createServiceWithWatchedLabels := func(svcName string, watchedLabels map[string]string) {
			// merge watchedLabels into service labels
			labels := map[string]string{"gloo": svcName}
			for key, val := range watchedLabels {
				labels[key] = val
			}
			// Write service
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:   svcName,
					Labels: labels,
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"gloo": svcName,
					},
					Ports: []corev1.ServicePort{{
						Port: helper.TestRunnerPort,
					}},
				},
			}
			service, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Create(ctx, service, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			createdServices = append(createdServices, service.Name)
		}

		getUpstream := func(svcname string) (*gloov1.Upstream, error) {
			upstreamName := fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, svcname, helper.TestRunnerPort)
			return upstreamClient.Read(testHelper.InstallNamespace, upstreamName, clients.ReadOpts{})
		}

		// Update the Gloo Discovery WatchLabels setting to the specified value
		setWatchLabels := func(watchLabels map[string]string) {
			kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
				Expect(settings.GetDiscovery()).NotTo(BeNil())
				settings.GetDiscovery().UdsOptions = &gloov1.Settings_DiscoveryOptions_UdsOptions{
					WatchLabels: watchLabels,
				}
			}, testHelper.InstallNamespace)
		}
		AfterEach(func() {
			for _, svcName := range createdServices {
				_ = kubeClient.CoreV1().Services(testHelper.InstallNamespace).Delete(ctx, svcName, metav1.DeleteOptions{})
			}

			setWatchLabels(nil)
		})

		It("should preserve discovery", func() {
			createServicesForPod(helper.TestrunnerName, helper.TestRunnerPort)

			for _, svc := range createdServices {
				Eventually(func() (*gloov1.Upstream, error) { return getUpstream(svc) }, "15s", "0.5s").ShouldNot(BeNil())
				// now set subset config on an upstream:
				Eventually(func() error {
					upstream, err := getUpstream(svc)
					if err != nil {
						return err
					}
					upstream.UpstreamType.(*gloov1.Upstream_Kube).Kube.ServiceSpec = &gloov1plugins.ServiceSpec{
						PluginType: &gloov1plugins.ServiceSpec_Grpc{
							Grpc: &grpcv1.ServiceSpec{},
						},
					}
					_, err = upstreamClient.Write(upstream, clients.WriteOpts{OverwriteExisting: true})
					return err
				}, "10s", "0.5s").ShouldNot(HaveOccurred())
			}

			// chill for a few letting discovery reconcile
			time.Sleep(time.Second * 10)

			// validate that all subset settings are still there
			for _, svc := range createdServices {
				// now set subset config on an upstream:
				up, _ := getUpstream(svc)
				spec := up.GetKube().GetServiceSpec()
				Expect(spec).ToNot(BeNil())
				Expect(spec.GetGrpc()).ToNot(BeNil())
			}
		})

		It("Discovers upstream with label that matches watched labels", func() {
			watchedKey := "A"
			watchedValue := "B"
			watchedLabels := map[string]string{watchedKey: watchedValue}
			setWatchLabels(watchedLabels)

			svcName := "uds-test-service"
			createServiceWithWatchedLabels(svcName, watchedLabels)

			Eventually(func() (*gloov1.Upstream, error) {
				return getUpstream(svcName)
			}, "15s", "0.5s").ShouldNot(BeNil())
		})

		It("Does not discover upstream with no label when watched labels are set", func() {
			watchedKey := "A"
			watchedValue := "B"
			watchedLabels := map[string]string{watchedKey: watchedValue}
			setWatchLabels(watchedLabels)

			svcName := "uds-test-service"
			createServiceWithWatchedLabels(svcName, nil)

			Consistently(func() error {
				_, err := getUpstream(svcName)
				return err
			}, "15s", "0.5s").Should(HaveOccurred())
		})

		It("Does not discover upstream with mismatched label value", func() {
			watchedKey := "A"
			watchedValue := "B"
			unwatchedValue := "C"
			watchedLabels := map[string]string{watchedKey: watchedValue}
			setWatchLabels(watchedLabels)

			svcName := "uds-test-service"
			unwatchedLabels := map[string]string{watchedKey: unwatchedValue}
			createServiceWithWatchedLabels(svcName, unwatchedLabels)

			Consistently(func() error {
				_, err := getUpstream(svcName)
				return err
			}, "15s", "0.5s").Should(HaveOccurred())
		})
	})

	Context("tcp", func() {

		var (
			defaultGateway *gatewayv1.Gateway
			httpEcho       helper.TestRunner
			usRef          *core.ResourceRef
			clusterIp      string

			tcpPort = corev1.ServicePort{
				Name:       "tcp-proxy",
				Port:       int32(defaults2.TcpPort),
				TargetPort: intstr.FromInt(int(defaults2.TcpPort)),
				Protocol:   "TCP",
			}

			initializeTcpGateway = func(host *gloov1.TcpHost) {
				defaultGateway = defaults.DefaultTcpGateway(testHelper.InstallNamespace)
				tcpGateway := defaultGateway.GetTcpGateway()
				Expect(tcpGateway).NotTo(BeNil())
				tcpGateway.TcpHosts = []*gloov1.TcpHost{host}
				Eventually(func() error {
					_, err := gatewayClient.Write(defaultGateway, clients.WriteOpts{})
					return err
				}, "15s", "0.5s").ShouldNot(HaveOccurred())
			}
		)

		BeforeEach(func() {
			var err error

			httpEcho, err = helper.NewEchoHttp(testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(httpEcho.Deploy(time.Minute)).NotTo(HaveOccurred())
			gwSvc, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Get(ctx, gatewayProxy, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			clusterIp = gwSvc.Spec.ClusterIP
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
			_, err = kubeClient.CoreV1().Services(testHelper.InstallNamespace).Update(ctx, gwSvc, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())

			usRef = &core.ResourceRef{
				Namespace: testHelper.InstallNamespace,
				Name:      kubernetes2.UpstreamName(testHelper.InstallNamespace, helper.HttpEchoName, helper.HttpEchoPort),
			}
		})

		AfterEach(func() {
			Expect(gatewayClient.Delete(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.DeleteOpts{})).NotTo(HaveOccurred())
			gwSvc, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Get(ctx, gatewayProxy, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			ports := make([]corev1.ServicePort, 0, len(gwSvc.Spec.Ports))
			for _, v := range gwSvc.Spec.Ports {
				if v.Name != tcpPort.Name {
					ports = append(ports, v)
				}
			}
			gwSvc.Spec.Ports = ports
			_, err = kubeClient.CoreV1().Services(testHelper.InstallNamespace).Update(ctx, gwSvc, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(httpEcho.Terminate()).NotTo(HaveOccurred())
			kubeClient.CoreV1().Services(testHelper.InstallNamespace).Delete(ctx, helper.HttpEchoName, metav1.DeleteOptions{})
		})

		It("correctly routes to the service (tcp)", func() {

			host := &gloov1.TcpHost{
				Name: "one",
				Destination: &gloov1.TcpHost_TcpAction{
					Destination: &gloov1.TcpHost_TcpAction_Single{
						Single: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Kube{
								Kube: &gloov1.KubernetesServiceDestination{
									Ref: &core.ResourceRef{
										Name:      helper.HttpEchoName,
										Namespace: testHelper.InstallNamespace,
									},
									Port: uint32(helper.HttpEchoPort),
								},
							},
						},
					},
				},
			}

			initializeTcpGateway(host)

			// wait for default gateway to be created
			Eventually(func() *gatewayv1.Gateway {
				gw, _ := gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{Ctx: ctx})
				return gw
			}, "15s", "0.5s").Should(Not(BeNil()))

			// wait for the expected proxy configuration to be accepted
			Eventually(func() error {
				proxy, err := proxyClient.Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				if err != nil {
					return err
				}

				proxyStatus := statusClient.GetStatus(proxy)
				if proxyStatus.GetState() != core.Status_Accepted {
					return eris.Errorf("unexpected proxy state: %v. Reason: %v", proxyStatus.GetState(), proxyStatus.GetReason())
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
									if svcDest.Ref.Name == helper.HttpEchoName &&
										svcDest.Ref.Namespace == testHelper.InstallNamespace &&
										svcDest.Port == uint32(helper.HttpEchoPort) {
										return nil
									}
								}
							}
						}
					}
				}

				return eris.Errorf("proxy did not contain expected route")
			}, "15s", "0.5s").Should(BeNil())

			responseString := fmt.Sprintf(`"hostname":"%s"`, gatewayProxy)

			httpEcho.CurlEventuallyShouldOutput(helper.CurlOpts{
				Protocol:          "http",
				Service:           gatewayProxy,
				Port:              int(defaultGateway.BindPort),
				ConnectionTimeout: 10,
				Verbose:           true,
			}, responseString, 1, 30*time.Second)
		})

		It("correctly routes to the service (tcp/tls)", func() {
			// Create secret to use for ssl routing
			createdSecret, err := kubeClient.CoreV1().Secrets(testHelper.InstallNamespace).Create(ctx, helpers.GetKubeSecret("secret", testHelper.InstallNamespace), metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			host := &gloov1.TcpHost{
				Name: "one",
				Destination: &gloov1.TcpHost_TcpAction{
					Destination: &gloov1.TcpHost_TcpAction_ForwardSniClusterName{
						ForwardSniClusterName: &empty.Empty{},
					},
				},
				SslConfig: &gloov1.SslConfig{
					// Use the translated cluster name as the SNI domain so envoy uses that in the cluster field
					SniDomains: []string{translator.UpstreamToClusterName(usRef)},
					SslSecrets: &gloov1.SslConfig_SecretRef{
						SecretRef: &core.ResourceRef{
							Name:      createdSecret.GetName(),
							Namespace: createdSecret.GetNamespace(),
						},
					},
					// Force http1, as defaulting to 2 fails. The service in question is an http1 service, but as this
					// is a standard TCP connection envoy does not know that, so it must rely on ALPN to figure that out.
					// However, by default the ALPN is set to []string{"h2", "http/1.1"} which favors http2.
					AlpnProtocols: []string{"http/1.1"},
				},
			}

			initializeTcpGateway(host)

			// wait for default gateway to be created
			Eventually(func() *gatewayv1.Gateway {
				gw, _ := gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{Ctx: ctx})
				return gw
			}, "15s", "0.5s").Should(Not(BeNil()))

			// wait for the expected proxy configuration to be accepted
			Eventually(func() (*empty.Empty, error) {
				proxy, err := proxyClient.Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				if err != nil {
					return nil, err
				}

				proxyStatus := statusClient.GetStatus(proxy)
				if proxyStatus.GetState() != core.Status_Accepted {
					return nil, eris.New("proxy not in accepted state")
				}

				for _, l := range proxy.Listeners {
					tcpListener := l.GetTcpListener()
					if tcpListener == nil {
						continue
					}
					for _, tcph := range tcpListener.TcpHosts {
						if action := tcph.GetDestination(); action != nil {
							return action.GetForwardSniClusterName(), nil
						}
					}
				}
				return nil, eris.New("proxy has no active listeners")
			}, "15s", "0.5s").Should(MatchProto(&empty.Empty{}))

			responseString := fmt.Sprintf(`"hostname":"%s"`, translator.UpstreamToClusterName(usRef))

			httpEcho.CurlEventuallyShouldOutput(helper.CurlOpts{
				Protocol:          "https",
				Sni:               translator.UpstreamToClusterName(usRef),
				Service:           clusterIp,
				Port:              int(defaultGateway.BindPort),
				ConnectionTimeout: 10,
				SelfSigned:        true,
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
			redPod, err = kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			pod.Labels["text"] = "blue"
			pod.Spec.Containers[0].Args = []string{"-text=\"blue-pod\""}
			bluePod, err = kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// green pod - no label
			delete(pod.Labels, "text")
			pod.Spec.Containers[0].Args = []string{"-text=\"green-pod\""}
			greenPod, err = kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Create(ctx, pod, metav1.CreateOptions{})
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
			service, err = kubeClient.CoreV1().Services(testHelper.InstallNamespace).Create(ctx, service, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if redPod != nil {
				err := kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Delete(ctx, redPod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}
			if bluePod != nil {
				err := kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Delete(ctx, bluePod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}
			if greenPod != nil {
				err := kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Delete(ctx, greenPod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}
			if service != nil {
				err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Delete(ctx, service.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
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

			// Ensure the redblue service is deleted
			helpers.EventuallyObjectDeleted(func() (client.Object, error) {
				return kubeClient.CoreV1().Services(testHelper.InstallNamespace).Get(ctx, service.Name, metav1.GetOptions{})
			}, "15s", ".5s")

			Eventually(func() error {
				coloredPods, err := kubeClient.CoreV1().Pods(testHelper.InstallNamespace).List(ctx,
					metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": "redblue"}).String()})
				if err != nil {
					return err
				}
				vsList, err := virtualServiceClient.List(vs.GetMetadata().Namespace, clients.ListOpts{Ctx: ctx})
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
				return eris.Errorf("expected all test resources to have been deleted but found: "+
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

			var upstreamRef *core.ResourceRef
			// upstream write might error on a conflict so try it a few times
			// I use eventually so it will wait a bit between retries.
			Eventually(func() error {
				upstream, _ := getUpstream()
				upstream.UpstreamType.(*gloov1.Upstream_Kube).Kube.SubsetSpec = &gloov1plugins.SubsetSpec{
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
				Metadata: &core.Metadata{
					Name:      "test",
					Namespace: testHelper.InstallNamespace,
				},
				Destinations: []*gloov1.WeightedDestination{
					{
						Weight: 1,
						Destination: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: upstreamRef,
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
								Upstream: upstreamRef,
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
								Upstream: upstreamRef,
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

			vs, err = virtualServiceClient.Write(&gatewayv1.VirtualService{
				Metadata: &core.Metadata{
					Name:      "vs",
					Namespace: testHelper.InstallNamespace,
				},
				VirtualHost: &gatewayv1.VirtualHost{
					Domains: []string{"*"},
					Routes: []*gatewayv1.Route{
						{
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/red",
								},
							}},
							Action: &gatewayv1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: upstreamRef,
											},
											Subset: &gloov1.Subset{
												Values: map[string]string{"text": "red"},
											},
										},
									},
								},
							},
						}, {
							Action: &gatewayv1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_UpstreamGroup{
										UpstreamGroup: ugref,
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
			Eventually(func() (*gatewayv1.Gateway, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			}, "15s", "0.5s").Should(Not(BeNil()))

			// make sure we get all three upstreams:
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

			// now make sure we only get the red pod
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

		Context("rejects bad resources", func() {

			// specifically avoiding using a DescribeTable here in order to avoid reinstalling
			// for every test case
			type testCase struct {
				resourceYaml, expectedErr string
			}

			testValidation := func(yaml, expectedErr string) {
				out, err := install.KubectlApplyOut([]byte(yaml))

				testValidationDidError := func() {
					ExpectWithOffset(1, err).To(HaveOccurred())
					ExpectWithOffset(1, string(out)).To(ContainSubstring(expectedErr))
				}

				testValidationDidSucceed := func() {
					ExpectWithOffset(1, err).NotTo(HaveOccurred())
					// To ensure that we do not leave artifacts between tests
					// we cleanup the resource after it is accepted
					err = install.KubectlDelete([]byte(yaml))
					ExpectWithOffset(1, err).NotTo(HaveOccurred())
				}

				if expectedErr == "" {
					testValidationDidSucceed()
				} else {
					testValidationDidError()
				}
			}

			Context("gateway", func() {

				It("rejects bad resources", func() {
					testCases := []testCase{
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
							// This is handled by validation schemas now
							expectedErr: `ValidationError(VirtualService.spec): unknown field "virtualHoost" in io.solo.gateway.v1.VirtualService.spec`,
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
    domains:
     - unique1
    routes:
      - matchers:
        - methods:
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
    domains:
     - unique2
    routes:
      - matchers:
        - exact: /delegated-nonprefix  # not allowed
        delegateAction:
          name: does-not-exist # also not allowed, but caught later
          namespace: anywhere
`,
							expectedErr: gwtranslator.MissingPrefixErr.Error(),
						},
					}

					for _, tc := range testCases {
						testValidation(tc.resourceYaml, tc.expectedErr)
					}
				})

			})

			Context("gloo", func() {

				var (
					// Validation of Gloo resources requires that a Proxy resource exist
					// Therefore, before the tests start, we must create valid resources that produce a Proxy
					placeholderVs *gatewayv1.VirtualService
				)

				BeforeEach(func() {
					dest := &gloov1.Destination{
						DestinationType: &gloov1.Destination_Upstream{
							Upstream: &core.ResourceRef{
								Namespace: testHelper.InstallNamespace,
								Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
							},
						},
					}

					placeholderVs = getVirtualService(dest, nil)

					_, err := virtualServiceClient.Write(placeholderVs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
						return proxyClient.Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
					})

					// Set the validation settings to be as strict as possible so that we can trigger
					// rejections by just producing a warning on the resource
					kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
						Expect(settings.GetGateway().GetValidation()).NotTo(BeNil())
						settings.GetGateway().GetValidation().AllowWarnings = &wrappers.BoolValue{Value: false}
					}, testHelper.InstallNamespace)
				})

				AfterEach(func() {
					kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
						Expect(settings.GetGateway().GetValidation()).NotTo(BeNil())
						settings.GetGateway().GetValidation().AllowWarnings = &wrappers.BoolValue{Value: true}
					}, testHelper.InstallNamespace)

					err := virtualServiceClient.Delete(placeholderVs.GetMetadata().GetNamespace(), placeholderVs.GetMetadata().GetName(), clients.DeleteOpts{})
					Expect(err).NotTo(HaveOccurred())
				})

				It("rejects bad resources", func() {
					testCases := []testCase{{
						resourceYaml: `
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: invalid-upstream
  namespace: gloo-system
spec:
  static:
    hosts:
      - addr: ~
`,
						expectedErr: "addr cannot be empty for host\n",
					}}

					for _, tc := range testCases {
						testValidation(tc.resourceYaml, tc.expectedErr)
					}
				})

			})
		})

		It("rejects invalid inja template in transformation", func() {
			injaTransform := `{% if default(data.error.message, "") != "" %}400{% else %}{{ header(":status") }}{% endif %}`
			t := &glootransformation.Transformations{
				ClearRouteCache: true,
				ResponseTransformation: &glootransformation.Transformation{
					TransformationType: &glootransformation.Transformation_TransformationTemplate{
						TransformationTemplate: &transformation.TransformationTemplate{
							Headers: map[string]*transformation.InjaTemplate{
								":status": {Text: injaTransform},
							},
						},
					},
				},
			}

			dest := &gloov1.Destination{
				DestinationType: &gloov1.Destination_Upstream{
					Upstream: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
					},
				},
			}

			vs := getVirtualService(dest, nil)
			vs.VirtualHost.Options = &gloov1.VirtualHostOptions{Transformations: t}

			_, err := virtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())

			err = virtualServiceClient.Delete(vs.Metadata.Namespace, vs.Metadata.Name, clients.DeleteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())

			// trim trailing "}", which should invalidate our inja template
			t.ResponseTransformation.GetTransformationTemplate().Headers[":status"].Text = strings.TrimSuffix(injaTransform, "}")

			_, err = virtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
			Expect(err).To(MatchError(ContainSubstring("Failed to parse response template: Failed to parse " +
				"header template ':status': [inja.exception.parser_error] expected statement close, got '%'")))
		})

		Context("disable_transformation_validation is set", func() {

			BeforeEach(func() {
				kube2e.UpdateDisableTransformationValidationSetting(ctx, true, testHelper.InstallNamespace)
			})

			AfterEach(func() {
				kube2e.UpdateDisableTransformationValidationSetting(ctx, false, testHelper.InstallNamespace)
			})

			It("will not reject invalid transformation", func() {
				// this inja template is invalid since it is missing a trailing "}",
				injaTransform := `{% if default(data.error.message, "") != "" %}400{% else %}{{ header(":status") }}{% endif %`
				t := &glootransformation.Transformations{
					ClearRouteCache: true,
					ResponseTransformation: &glootransformation.Transformation{
						TransformationType: &glootransformation.Transformation_TransformationTemplate{
							TransformationTemplate: &transformation.TransformationTemplate{
								Headers: map[string]*transformation.InjaTemplate{
									":status": {Text: injaTransform},
								},
							},
						},
					},
				}

				dest := &gloov1.Destination{
					DestinationType: &gloov1.Destination_Upstream{
						Upstream: &core.ResourceRef{
							Namespace: testHelper.InstallNamespace,
							Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
						},
					},
				}

				vs := getVirtualService(dest, nil)
				vs.VirtualHost.Options = &gloov1.VirtualHostOptions{Transformations: t}

				// give settings a chance to propagate
				Eventually(func() error {
					_, err := virtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
					return err
				}, "5s", "0.1s").ShouldNot(HaveOccurred())
				helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					return virtualServiceClient.Read(testHelper.InstallNamespace, vs.GetMetadata().GetName(), clients.ReadOpts{Ctx: ctx})
				})

				err := virtualServiceClient.Delete(vs.Metadata.Namespace, vs.Metadata.Name, clients.DeleteOpts{Ctx: ctx})
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("compressed spec is working", func() {
		var (
			settingsClient gloov1.SettingsClient
		)

		BeforeEach(func() {
			settingsClient = clienthelpers.MustSettingsClient(ctx)

			settingsList, err := settingsClient.List(testHelper.InstallNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(settingsList).To(HaveLen(1))
			settings := settingsList[0]

			if settings.Gateway == nil {
				settings.Gateway = &gloov1.GatewayOptions{
					CompressedProxySpec: true,
				}
			}

			_, err = settingsClient.Write(settings, clients.WriteOpts{
				OverwriteExisting: true,
			})
			Expect(err).NotTo(HaveOccurred())

		})

		AfterEach(func() {
			settingsList, err := settingsClient.List(testHelper.InstallNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(settingsList).To(HaveLen(1))
			settings := settingsList[0]
			settings.Gateway.CompressedProxySpec = false
			_, err = settingsClient.Write(settings, clients.WriteOpts{
				OverwriteExisting: true,
			})
			Expect(err).NotTo(HaveOccurred())

			err = virtualServiceClient.Delete(testHelper.InstallNamespace, "vs", clients.DeleteOpts{IgnoreNotExist: true})
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
			Eventually(func() (*gatewayv1.Gateway, error) {
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
			}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)
		})

	})

	FContext("matchable hybrid gateway", func() {

		var (
			hybridProxyServicePort = corev1.ServicePort{
				Name:       "hybrid-proxy",
				Port:       int32(defaults2.HybridPort),
				TargetPort: intstr.FromInt(int(defaults2.HybridPort)),
				Protocol:   "TCP",
			}
		)

		exposePortOnGwProxyService := func(servicePort corev1.ServicePort) {
			gwSvc, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Get(ctx, gatewayProxy, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Append servicePort if not found already
			found := false
			for _, v := range gwSvc.Spec.Ports {
				if v.Name == hybridProxyServicePort.Name || v.Port == hybridProxyServicePort.Port {
					found = true
					break
				}
			}
			if !found {
				gwSvc.Spec.Ports = append(gwSvc.Spec.Ports, hybridProxyServicePort)
			}

			_, err = kubeClient.CoreV1().Services(testHelper.InstallNamespace).Update(ctx, gwSvc, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			exposePortOnGwProxyService(hybridProxyServicePort)
		})

		It("works", func() {
			dest := &gloov1.Destination{
				DestinationType: &gloov1.Destination_Upstream{
					Upstream: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
					},
				},
			}
			// Create a VS that routes to a destination
			Eventually(func() error {
				_, err := virtualServiceClient.Write(getVirtualService(dest, nil), clients.WriteOpts{})
				return err
			}).ShouldNot(HaveOccurred())

			// wait for default gateway to be created
			// We have default gateways as part of the gloo install and now that a vs is associated with one,
			// the translation loop should process it and write a status to it
			defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			})

			// Create a MatchableHttpGateway
			matchableHttpGateway :=  &gatewayv1.MatchableHttpGateway{
				Metadata: &core.Metadata{
					Name:        "matchable-http-gateway",
					Namespace:   testHelper.InstallNamespace,
				},
				HttpGateway: &gatewayv1.HttpGateway{
					// match all virtual services
				},
			}
			_, err := httpGatewayClient.Write(matchableHttpGateway, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// Create a HybridGateway that references that MatchableHttpGateway
			hybridGateway := &gatewayv1.Gateway{
				Metadata: &core.Metadata{
					Name:        fmt.Sprintf("%s-hybrid", defaults.GatewayProxyName),
					Namespace:   testHelper.InstallNamespace,
				},
				GatewayType: &gatewayv1.Gateway_HybridGateway{
					HybridGateway: &gatewayv1.HybridGateway{
						DelegatedHttpGateways: &gatewayv1.DelegatedHttpGateway{
							SelectionType: &gatewayv1.DelegatedHttpGateway_Ref{
								Ref: &core.ResourceRef{
									Name:      matchableHttpGateway.GetMetadata().GetName(),
									Namespace: matchableHttpGateway.GetMetadata().GetNamespace(),
								},
							},
						},
					},
				},
				ProxyNames:    []string{defaults.GatewayProxyName},
				BindAddress:   defaults.GatewayBindAddress,
				BindPort:      defaults2.HybridPort,
				UseProxyProto: &wrappers.BoolValue{Value: false},
			}

			// Write the HybridGateway and wait for it to be accepted
			Eventually(func() error {
				_, err := gatewayClient.Write(hybridGateway, clients.WriteOpts{})
				return err
			}, "15s", "0.5s").ShouldNot(HaveOccurred())
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, hybridGateway.Metadata.Name, clients.ReadOpts{})
			})

			// destination reachable via HttpGateway
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 5, // this is important, as sometimes curl hangs
				WithoutStats:      false,
				Verbose: true,
			}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)

			// destination reachable via HybridGateway
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              int(hybridProxyServicePort.Port),
				ConnectionTimeout: 5, // this is important, as sometimes curl hangs
				WithoutStats:      false,
				Verbose: true,
			}, kube2e.SimpleTestRunnerHttpResponse, 1, 60*time.Second, 1*time.Second)
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

func withName(name string, vs *gatewayv1.VirtualService) *gatewayv1.VirtualService {
	vs.Metadata.Name = name
	return vs
}

func withDomains(domains []string, vs *gatewayv1.VirtualService) *gatewayv1.VirtualService {
	vs.VirtualHost.Domains = domains
	return vs
}

func withRoute(route *gatewayv1.Route, vs *gatewayv1.VirtualService) *gatewayv1.VirtualService {
	vs.VirtualHost.Routes = append([]*gatewayv1.Route{route}, vs.VirtualHost.Routes...)
	return vs
}

func getVirtualService(dest *gloov1.Destination, sslConfig *gloov1.SslConfig) *gatewayv1.VirtualService {
	return getVirtualServiceWithRoute(getRouteWithDest(dest, "/"), sslConfig)
}

func getVirtualServiceWithRoute(route *gatewayv1.Route, sslConfig *gloov1.SslConfig) *gatewayv1.VirtualService {
	return &gatewayv1.VirtualService{
		Metadata: &core.Metadata{
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

func getRouteTable(name string, labels map[string]string, route *gatewayv1.Route) *gatewayv1.RouteTable {
	return &gatewayv1.RouteTable{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: testHelper.InstallNamespace,
			Labels:    labels,
		},
		Routes: []*gatewayv1.Route{route},
	}
}

func getRouteWithDirectResponse(message string, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		Matchers: []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: path,
			},
		}},
		Action: &gatewayv1.Route_DirectResponseAction{
			DirectResponseAction: &gloov1.DirectResponseAction{
				Status: 200,
				Body:   message,
			},
		},
	}
}

func getRouteWithDest(dest *gloov1.Destination, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		Matchers: []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: path,
			},
		}},
		Action: &gatewayv1.Route_RouteAction{
			RouteAction: &gloov1.RouteAction{
				Destination: &gloov1.RouteAction_Single{
					Single: dest,
				},
			},
		},
	}
}

func getRouteWithDelegateRef(delegate string, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		Matchers: []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: path,
			},
		}},
		Action: &gatewayv1.Route_DelegateAction{
			DelegateAction: &gatewayv1.DelegateAction{
				DelegationType: &gatewayv1.DelegateAction_Ref{
					Ref: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      delegate,
					},
				},
			},
		},
	}
}

func getRouteWithDelegateSelector(labels map[string]string, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		Matchers: []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: path,
			},
		}},
		Action: &gatewayv1.Route_DelegateAction{
			DelegateAction: &gatewayv1.DelegateAction{
				DelegationType: &gatewayv1.DelegateAction_Selector{
					Selector: &gatewayv1.RouteTableSelector{
						Namespaces: []string{testHelper.InstallNamespace},
						Labels:     labels,
					},
				},
			},
		},
	}
}

func addPrefixRewrite(route *gatewayv1.Route, rewrite string) *gatewayv1.Route {
	route.Options = &gloov1.RouteOptions{PrefixRewrite: &wrappers.StringValue{Value: rewrite}}
	return route
}

func petstore(namespace string) (*v1.Deployment, *corev1.Service) {
	deployment := fmt.Sprintf(`
##########################
#                        #
#        Example         #
#        Service         #
#                        #
#                        #
##########################
# petstore service
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: petstore
  name: petstore
  namespace: %s
spec:
  selector:
    matchLabels:
      app: petstore
  replicas: 1
  template:
    metadata:
      labels:
        app: petstore
    spec:
      containers:
      - image: soloio/petstore-example:latest
        name: petstore
        ports:
        - containerPort: 8080
          name: http
`, namespace)

	var dep v1.Deployment
	err := yaml.Unmarshal([]byte(deployment), &dep)
	Expect(err).NotTo(HaveOccurred())

	service := fmt.Sprintf(`
---
apiVersion: v1
kind: Service
metadata:
  name: petstore
  namespace: %s
  labels:
    service: petstore
spec:
  ports:
  - port: 8080
    protocol: TCP
  selector:
    app: petstore
`, namespace)

	var svc corev1.Service
	err = yaml.Unmarshal([]byte(service), &svc)
	Expect(err).NotTo(HaveOccurred())

	return &dep, &svc
}
