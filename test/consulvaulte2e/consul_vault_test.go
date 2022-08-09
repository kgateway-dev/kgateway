package consulvaulte2e_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/rest"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	writeNamespace     = defaults.GlooSystem
	customSecretEngine = "custom-secret-engine"
)

var _ = Describe("Consul + Vault Configuration Happy Path e2e", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc

		consulInstance *services.ConsulInstance
		vaultInstance  *services.VaultInstance
		envoyInstance  *services.EnvoyInstance

		testClients services.TestClients
		settingsDir string

		svc1 *v1helpers.TestUpstream
		err  error
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		defaults.HttpPort = services.NextBindPort()

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
		err = vaultInstance.EnableSecretEngine(customSecretEngine)
		Expect(err).NotTo(HaveOccurred())

		// Start Gloo
		watchNamespaces := []string{"default", writeNamespace}
		settingsDir, err = setupSettingsDirectory(watchNamespaces)
		Expect(err).NotTo(HaveOccurred())
		runOptions := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: watchNamespaces,
			WhatToRun: services.What{
				DisableFds: false,
				DisableUds: false,
			},
			Settings: &gloov1.Settings{
				ConfigSource: &gloov1.Settings_ConsulKvSource{
					ConsulKvSource: &gloov1.Settings_ConsulKv{},
				},
				SecretSource: &gloov1.Settings_VaultSecretSource{
					VaultSecretSource: &gloov1.Settings_VaultSecrets{
						Address:    vaultInstance.Address(),
						Token:      vaultInstance.Token(),
						PathPrefix: customSecretEngine,
						RootKey:    bootstrap.DefaultRootKey,
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
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, runOptions)

		err = helpers.WriteDefaultGateways(writeNamespace, testClients.GatewayClient)
		Expect(err).NotTo(HaveOccurred(), "Should be able to write the default gateways")

		// Start Envoy
		envoyInstance = envoyFactory.MustEnvoyInstance()
		err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		// Setup a simple web application locally
		svc1 = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		// Start petstore
		petstorePort := 1234
		go func() {
			defer GinkgoRecover()

			petstoreErr := services.RunPetstore(ctx, petstorePort)
			if petstoreErr != nil {
				Expect(petstoreErr.Error()).To(ContainSubstring("http: Server closed"))
			}
		}()

		// Register services with consul
		err = consulInstance.RegisterService("my-svc", "my-svc-1", envoyInstance.LocalAddr(), []string{"svc", "1"}, svc1.Port)
		Expect(err).NotTo(HaveOccurred())

		err = consulInstance.RegisterService("petstore", "petstore-1", envoyInstance.LocalAddr(), []string{"svc", "petstore"}, uint32(petstorePort))
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err = consulInstance.Clean()
		Expect(err).NotTo(HaveOccurred())

		err = vaultInstance.Clean()
		Expect(err).NotTo(HaveOccurred())

		envoyInstance.Clean()

		err = os.RemoveAll(settingsDir)
		Expect(err).NotTo(HaveOccurred())

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

		_, err = testClients.SecretClient.Write(secret, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		vs := makeSslVirtualService(writeNamespace, secret.Metadata.Ref())
		vs, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Wait for the proxy to be accepted. this can take up to 40 seconds, as the vault snapshot
		// updates every 30 seconds.
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
		}, "60s", ".2s")

		v1helpers.TestUpstreamReachable(defaults.HttpsPort, svc1, &cert)
	})

	FIt("can do function routing with consul services", func() {
		us := &core.ResourceRef{Namespace: writeNamespace, Name: "petstore"}

		vs := makeFunctionRoutingVirtualService(writeNamespace, us, "findPetById")

		vs, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Wait for the proxy to be accepted.
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
		}, "60s", ".2s")

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

func setupSettingsDirectory(watchNamespaces []string) (string, error) {
	settingsDir, err := ioutil.TempDir("", "")
	Expect(err).NotTo(HaveOccurred())

	var requiredPaths []string

	for _, ns := range watchNamespaces {
		// must create a directory for artifacts so gloo doesn't error
		requiredPaths = append(requiredPaths, filepath.Join(settingsDir, gloov1.ArtifactCrd.Plural, ns))
	}

	for _, requiredPath := range requiredPaths {
		if err := os.MkdirAll(requiredPath, 0755); err != nil {
			return "", err
		}
	}

	return settingsDir, nil
}
