package consulvaulte2e_test

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"

	"github.com/gogo/protobuf/types"
	consulapi "github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	gatewaysetup "github.com/solo-io/gloo/projects/gateway/pkg/setup"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/setup"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	"github.com/solo-io/gloo/test/v1helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Consul + Vault Configuration Happy Path e2e", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		consulInstance *services.ConsulInstance
		vaultInstance  *services.VaultInstance
		envoyInstance  *services.EnvoyInstance
		envoyPort      uint32
		svc1, svc2     *v1helpers.TestUpstream
		err            error
		settingsDir    string

		consulClient    *consulapi.Client
		vaultClient     *vaultapi.Client
		consulResources factory.ResourceClientFactory
		vaultResources  factory.ResourceClientFactory
	)

	const writeNamespace = defaults.GlooSystem

	queryService := func() (string, error) {
		response, err := http.Get(fmt.Sprintf("http://localhost:%d", envoyPort))
		if err != nil {
			return "", err
		}
		//noinspection GoUnhandledErrorResult
		defer response.Body.Close()

		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		// Start Consul
		consulInstance, err = consulFactory.NewConsulInstance()
		Expect(err).NotTo(HaveOccurred())
		err = consulInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		// Start Vault
		vaultInstance, err = vaultFactory.NewVaultInstance()
		Expect(err).NotTo(HaveOccurred())
		err = vaultInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		// write settings telling Gloo to use consul/vault
		settingsDir, err = ioutil.TempDir("", "")
		Expect(err).NotTo(HaveOccurred())

		settings, err := writeSettings(settingsDir, writeNamespace)
		Expect(err).NotTo(HaveOccurred())

		consulClient, err = bootstrap.ConsulClientForSettings(settings)
		Expect(err).NotTo(HaveOccurred())

		vaultClient, err = bootstrap.VaultClientForSettings(settings.GetVaultSecretSource())
		Expect(err).NotTo(HaveOccurred())

		consulResources = &factory.ConsulResourceClientFactory{
			RootKey: bootstrap.RootKey,
			Consul:  consulClient,
		}
		vaultResources = &factory.VaultSecretClientFactory{
			Vault:   vaultClient,
			RootKey: bootstrap.RootKey,
		}

		// set flag for gloo to use settings dir
		err = flag.Set("dir", settingsDir)
		err = flag.Set("namespace", writeNamespace)
		Expect(err).NotTo(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			// Start Gloo
			err = setup.Main(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()
		go func() {
			defer GinkgoRecover()
			// Start Gateway
			err = gatewaysetup.Main(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()

		// Start Envoy
		envoyPort = uint32(defaults.HttpPort)
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.RunWithRole(writeNamespace+"~"+translator.GatewayProxyName, 9977)
		Expect(err).NotTo(HaveOccurred())

		// Run two simple web applications locally
		svc1 = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		svc2 = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		// Register services with consul
		err = consulInstance.RegisterService("my-svc", "my-svc-1", envoyInstance.GlooAddr, []string{"svc", "1"}, svc1.Port)
		Expect(err).NotTo(HaveOccurred())
		err = consulInstance.RegisterService("my-svc", "my-svc-2", envoyInstance.GlooAddr, []string{"svc", "1"}, svc2.Port)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if consulInstance != nil {
			err = consulInstance.Clean()
			Expect(err).NotTo(HaveOccurred())
		}
		if vaultInstance != nil {
			err = vaultInstance.Clean()
			Expect(err).NotTo(HaveOccurred())
		}
		if envoyInstance != nil {
			err = envoyInstance.Clean()
			Expect(err).NotTo(HaveOccurred())
		}
		os.RemoveAll(settingsDir)

		cancel()
	})

	It("can be configured using consul k-v", func() {

		proxyClient, err := gloov1.NewProxyClient(consulResources)
		Expect(err).NotTo(HaveOccurred())

		_, err = proxyClient.Write(getProxyWithConsulRoute(writeNamespace, envoyPort), clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Wait for proxy to be accepted
		Eventually(func() bool {
			proxy, err := proxyClient.Read(writeNamespace, "gateway-proxy", clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return false
			}
			return proxy.Status.State == core.Status_Accepted
		}, "10s", "0.2s").Should(BeTrue())

		By("requests are load balanced between the two services")
		Eventually(func() (<-chan *v1helpers.ReceivedRequest, error) {
			_, err := queryService()
			if err != nil {
				return svc1.C, err
			}
			return svc1.C, nil
		}, "10s", "0.2s").Should(Receive())

		Eventually(func() (<-chan *v1helpers.ReceivedRequest, error) {
			_, err := queryService()
			if err != nil {
				return svc2.C, err
			}
			return svc2.C, nil
		}, "10s", "0.2s").Should(Receive())

		By("update consul service definition")
		err = consulInstance.RegisterService("my-svc", "my-svc-2", envoyInstance.GlooAddr, []string{"svc", "2"}, svc2.Port)
		Expect(err).NotTo(HaveOccurred())

		// Wait a bit for the new endpoint information to propagate
		time.Sleep(3 * time.Second)

		// Service 2 does not match the tags on the route anymore, so we should get only requests from service 1
		Consistently(func() (<-chan *v1helpers.ReceivedRequest, error) {
			_, err := queryService()
			if err != nil {
				return svc1.C, err
			}
			return svc1.C, nil
		}, "2s", "0.2s").Should(Receive())
	})
	It("can read secrets using vault", func() {
		cert := helpers.Certificate()

		secret := &gloov1.Secret{
			Metadata: core.Metadata{
				Name:      "secret",
				Namespace: "default",
			},
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{
					CertChain:  cert,
					PrivateKey: helpers.PrivateKey(),
				},
			},
		}

		secretClient, err := gloov1.NewSecretClient(vaultResources)
		Expect(err).NotTo(HaveOccurred())

		_, err = secretClient.Write(secret, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		vsClient, err := v1.NewVirtualServiceClient(consulResources)
		Expect(err).NotTo(HaveOccurred())

		vs := makeSslVirtualService(secret.Metadata.Ref())

		vs, err = vsClient.Write(vs, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Wait for vs and gw to be accepted
		Eventually(func() (core.Status_State, error) {
			vs, err := vsClient.Read(vs.Metadata.Namespace, vs.Metadata.Name, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return 0, err
			}
			return vs.Status.State, nil
		}, "5s", "0.2s").Should(Equal(core.Status_Accepted))

		v1helpers.TestUpstreamReachable(defaults.HttpsPort, svc1, &cert)
	})
})

func getProxyWithConsulRoute(ns string, bindPort uint32) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: core.Metadata{
			Name:      "gateway-proxy",
			Namespace: ns,
		},
		Listeners: []*gloov1.Listener{{
			Name:        "listener",
			BindAddress: "::",
			BindPort:    bindPort,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: []*gloov1.VirtualHost{{
						Name:    "vh-1",
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
											DestinationType: &gloov1.Destination_Consul{
												Consul: &gloov1.ConsulServiceDestination{
													ServiceName: "my-svc",
													Tags:        []string{"svc", "1"},
												},
											},
										},
									},
								},
							},
						}},
					}},
				},
			},
		}},
	}
}

func makeSslVirtualService(secret core.ResourceRef) *v1.VirtualService {
	return &v1.VirtualService{
		Metadata: core.Metadata{
			Name:      "vs-ssl",
			Namespace: "default",
		},
		VirtualHost: &gloov1.VirtualHost{
			Name:    "virt1",
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
								DestinationType: &gloov1.Destination_Consul{
									Consul: &gloov1.ConsulServiceDestination{
										ServiceName: "my-svc",
										Tags:        []string{"svc", "1"},
									},
								},
							},
						},
					},
				},
			}},
		},
		SslConfig: &gloov1.SslConfig{
			SslSecrets: &gloov1.SslConfig_SecretRef{
				SecretRef: &core.ResourceRef{
					Name:      secret.Name,
					Namespace: secret.Namespace,
				},
			},
		},
	}
}

func writeSettings(settingsDir, writeNamespace string) (*gloov1.Settings, error) {
	settings := &gloov1.Settings{
		ConfigSource: &gloov1.Settings_ConsulKvSource{
			ConsulKvSource: &gloov1.Settings_ConsulKv{},
		},
		SecretSource: &gloov1.Settings_VaultSecretSource{
			VaultSecretSource: &gloov1.Settings_VaultSecrets{
				Address: "http://127.0.0.1:8200",
				Token:   "root",
			},
		},
		ArtifactSource: &gloov1.Settings_DirectoryArtifactSource{
			DirectoryArtifactSource: &gloov1.Settings_Directory{
				Directory: settingsDir,
			},
		},
		Consul: &gloov1.Settings_ConsulConfiguration{
			ServiceDiscovery: &gloov1.Settings_ConsulConfiguration_ServiceDiscoveryOptions{},
		},
		BindAddr:           "0.0.0.0:9977",
		RefreshRate:        types.DurationProto(time.Minute),
		DiscoveryNamespace: writeNamespace,
		Metadata:           core.Metadata{Namespace: writeNamespace, Name: "default"},
	}
	yam, err := protoutils.MarshalYAML(settings)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(settingsDir, writeNamespace), 0755); err != nil {
		return nil, err
	}
	// must create a directory for artifacts so gloo doesn't error
	if err := os.MkdirAll(filepath.Join(settingsDir, "artifacts", "default"), 0755); err != nil {
		return nil, err
	}
	return settings, ioutil.WriteFile(filepath.Join(settingsDir, writeNamespace, "default.yaml"), yam, 0644)
}
