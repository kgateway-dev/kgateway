package consulvaulte2e_test

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"

	fdssetup "github.com/solo-io/gloo/projects/discovery/pkg/fds/setup"
	udssetup "github.com/solo-io/gloo/projects/discovery/pkg/uds/setup"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/rest"

	consulapi "github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaysetup "github.com/solo-io/gloo/projects/gateway/pkg/setup"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/setup"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
		svc1           *v1helpers.TestUpstream
		err            error
		settingsDir    string

		consulClient    *consulapi.Client
		vaultClient     *vaultapi.Client
		consulResources factory.ResourceClientFactory
		vaultResources  factory.ResourceClientFactory

		petstorePort   int
		glooPort       int
		validationPort int
		restXdsPort    int
	)

	const writeNamespace = defaults.GlooSystem

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		glooPort = int(services.AllocateGlooPort())
		validationPort = int(services.AllocateGlooPort())
		restXdsPort = int(services.AllocateGlooPort())

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

		settings, err := writeSettings(settingsDir, glooPort, validationPort, restXdsPort, writeNamespace)
		Expect(err).NotTo(HaveOccurred())

		consulClient, err = bootstrap.ConsulClientForSettings(ctx, settings)
		Expect(err).NotTo(HaveOccurred())

		vaultClient, err = bootstrap.VaultClientForSettings(settings.GetVaultSecretSource())
		Expect(err).NotTo(HaveOccurred())

		consulResources = &factory.ConsulResourceClientFactory{
			RootKey: bootstrap.DefaultRootKey,
			Consul:  consulClient,
		}

		gatewayClient, err := v1.NewGatewayClient(ctx, consulResources)
		Expect(err).NotTo(HaveOccurred(), "Should be able to build the gateway client")
		err = helpers.WriteDefaultGateways(writeNamespace, gatewayClient)
		Expect(err).NotTo(HaveOccurred(), "Should be able to write the default gateways")

		vaultResources = &factory.VaultSecretClientFactory{
			Vault:   vaultClient,
			RootKey: bootstrap.DefaultRootKey,
		}

		// set flag for gloo to use settings dir
		err = flag.Set("dir", settingsDir)
		err = flag.Set("namespace", writeNamespace)
		Expect(err).NotTo(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			// Start Gloo
			err = setup.StartGlooInTest(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()
		go func() {
			defer GinkgoRecover()
			// Start Gateway
			err = gatewaysetup.Main(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()
		go func() {
			defer GinkgoRecover()
			// Start FDS
			err = fdssetup.Main(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()
		go func() {
			defer GinkgoRecover()
			// Start UDS
			err = udssetup.Main(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()

		// Start Envoy
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		envoyInstance.RestXdsPort = uint32(restXdsPort)
		err = envoyInstance.RunWithRole(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, glooPort)
		Expect(err).NotTo(HaveOccurred())

		// Run a simple web application locally
		svc1 = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		// Run the petstore locally
		petstorePort = 1234
		go func() {
			defer GinkgoRecover()
			// Start petstore
			err = services.RunPetstore(ctx, petstorePort)
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("http: Server closed"))
			}
		}()

		// Register services with consul
		err = consulInstance.RegisterService("my-svc", "my-svc-1", envoyInstance.GlooAddr, []string{"svc", "1"}, svc1.Port)
		Expect(err).NotTo(HaveOccurred())

		err = consulInstance.RegisterService("petstore", "petstore-1", envoyInstance.GlooAddr, []string{"svc", "petstore"}, uint32(petstorePort))
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

	It("can be configured using consul k-v and read secrets using vault", func() {
		cert := helpers.Certificate()

		secret := &gloov1.Secret{
			Metadata: &core.Metadata{
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

		secretClient, err := gloov1.NewSecretClient(ctx, vaultResources)
		Expect(err).NotTo(HaveOccurred())

		_, err = secretClient.Write(secret, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		vsClient, err := v1.NewVirtualServiceClient(ctx, consulResources)
		Expect(err).NotTo(HaveOccurred())

		proxyClient, err := gloov1.NewProxyClient(ctx, consulResources)
		Expect(err).NotTo(HaveOccurred())

		vs := makeSslVirtualService(writeNamespace, secret.Metadata.Ref())

		vs, err = vsClient.Write(vs, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Wait for vs and gw to be accepted
		Eventually(func() (core.Status_State, error) {
			readVs, err := vsClient.Read(vs.Metadata.Namespace, vs.Metadata.Name, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return 0, err
			}
			return readVs.GetStatus().GetState(), nil
		}, "60s", "0.2s").Should(Equal(core.Status_Accepted))

		// Wait for the proxy to be accepted. this can take up to 40 seconds, as the vault snapshot
		// updates every 30 seconds.
		Eventually(func() (core.Status_State, error) {
			proxy, err := proxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return 0, err
			}
			return proxy.GetStatus().GetState(), nil
		}, "60s", "0.2s").Should(Equal(core.Status_Accepted))

		v1helpers.TestUpstreamReachable(defaults.HttpsPort, svc1, &cert)
	})
	It("can do function routing with consul services", func() {

		vsClient, err := v1.NewVirtualServiceClient(ctx, consulResources)
		Expect(err).NotTo(HaveOccurred())

		proxyClient, err := gloov1.NewProxyClient(ctx, consulResources)
		Expect(err).NotTo(HaveOccurred())

		us := &core.ResourceRef{Namespace: "gloo-system", Name: "petstore"}

		vs := makeFunctionRoutingVirtualService(writeNamespace, us, "findPetById")

		vs, err = vsClient.Write(vs, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Wait for the proxy to be accepted.
		Eventually(func() (core.Status_State, error) {
			proxy, err := proxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return 0, err
			}
			return proxy.GetStatus().GetState(), nil
		}, "60s", "0.2s").Should(Equal(core.Status_Accepted))

		v1helpers.ExpectHttpOK(nil, nil, defaults.HttpPort,
			`[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
`)
	})
})

func makeSslVirtualService(vsNamespace string, secret *core.ResourceRef) *v1.VirtualService {
	return &v1.VirtualService{
		Metadata: &core.Metadata{
			Name:      "vs-ssl",
			Namespace: vsNamespace,
		},
		VirtualHost: &v1.VirtualHost{
			Domains: []string{"*"},
			Routes: []*v1.Route{{
				Action: &v1.Route_RouteAction{
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

func makeFunctionRoutingVirtualService(vsNamespace string, upstream *core.ResourceRef, funcName string) *v1.VirtualService {
	return &v1.VirtualService{
		Metadata: &core.Metadata{
			Name:      "vs-functions",
			Namespace: vsNamespace,
		},
		VirtualHost: &v1.VirtualHost{
			Domains: []string{"*"},
			Routes: []*v1.Route{{
				Action: &v1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Single{
							Single: &gloov1.Destination{
								DestinationType: &gloov1.Destination_Upstream{
									Upstream: upstream,
								},
								DestinationSpec: &gloov1.DestinationSpec{
									DestinationType: &gloov1.DestinationSpec_Rest{
										Rest: &rest.DestinationSpec{
											FunctionName: funcName,
										},
									},
								},
							},
						},
					},
				},
			}},
		},
	}
}

func writeSettings(settingsDir string, glooPort, validationPort, restXdsPort int, writeNamespace string) (*gloov1.Settings, error) {
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
		Discovery: &gloov1.Settings_DiscoveryOptions{
			FdsMode: gloov1.Settings_DiscoveryOptions_BLACKLIST,
		},
		Consul: &gloov1.Settings_ConsulConfiguration{
			ServiceDiscovery: &gloov1.Settings_ConsulConfiguration_ServiceDiscoveryOptions{},
		},
		Gloo: &gloov1.GlooOptions{
			XdsBindAddr:        fmt.Sprintf("0.0.0.0:%v", glooPort),
			ValidationBindAddr: fmt.Sprintf("0.0.0.0:%v", validationPort),
			RestXdsBindAddr:    fmt.Sprintf("0.0.0.0:%v", restXdsPort),
		},
		RefreshRate:        prototime.DurationToProto(time.Second * 1),
		DiscoveryNamespace: writeNamespace,
		Metadata:           &core.Metadata{Namespace: writeNamespace, Name: "default"},
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
