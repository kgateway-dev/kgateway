package setup_test

import (
	"context"
	"net"
	"os"
	"time"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"

	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
)

var _ = Describe("SetupSyncer", func() {

	var (
		settings *v1.Settings
		ctx      context.Context
		cancel   context.CancelFunc
		memcache memory.InMemoryResourceCache
	)
	newContext := func() {
		if cancel != nil {
			cancel()
		}
		ctx, cancel = context.WithCancel(context.Background())
		ctx = settingsutil.WithSettings(ctx, settings)

	}

	BeforeEach(func() {
		settings = &v1.Settings{
			RefreshRate: prototime.DurationToProto(time.Hour),
			Gloo: &v1.GlooOptions{
				XdsBindAddr:        getRandomAddr(),
				ValidationBindAddr: getRandomAddr(),
			},
			DiscoveryNamespace: "non-existent-namespace",
			WatchNamespaces:    []string{"non-existent-namespace"},
		}
		memcache = memory.NewInMemoryResourceCache()
		newContext()
	})
	AfterEach(func() {
		cancel()
	})
	Context("Setup", func() {
		setupTestGrpcClient := func() func() error {
			cc, err := grpc.DialContext(ctx, settings.Gloo.XdsBindAddr, grpc.WithInsecure(), grpc.FailOnNonTempDialError(true))
			Expect(err).NotTo(HaveOccurred())
			// setup a gRPC client to make sure connection is persistent across invocations
			client := reflectpb.NewServerReflectionClient(cc)
			req := &reflectpb.ServerReflectionRequest{
				MessageRequest: &reflectpb.ServerReflectionRequest_ListServices{
					ListServices: "*",
				},
			}
			clientstream, err := client.ServerReflectionInfo(context.Background())
			Expect(err).NotTo(HaveOccurred())
			err = clientstream.Send(req)
			go func() {
				for {
					_, err := clientstream.Recv()
					if err != nil {
						return
					}
				}
			}()
			Expect(err).NotTo(HaveOccurred())
			return func() error { return clientstream.Send(req) }
		}
		Context("XDS tests", func() {

			It("setup can be called twice", func() {
				setup := NewSetupFunc()

				err := setup(ctx, nil, memcache, settings)
				Expect(err).NotTo(HaveOccurred())

				testFunc := setupTestGrpcClient()

				newContext()
				err = setup(ctx, nil, memcache, settings)
				Expect(err).NotTo(HaveOccurred())

				// give things a chance to react
				time.Sleep(time.Second)

				// make sure that xds snapshot was not restarted
				err = testFunc()
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Context("Extensions tests", func() {
			var (
				plugin1 = &dummyPlugin{}
				plugin2 = &dummyPlugin{}

				opts = bootstrap.Opts{}
			)
			It("should return plugins", func() {
				registryPlugins := func(opts bootstrap.Opts) []plugins.Plugin { return nil }

				extensions := Extensions{
					PluginExtensions: []plugins.Plugin{
						plugin1,
						plugin2,
					},
				}

				pluginFuncs := GetPluginsWithExtensionsAndRegistry(opts, registryPlugins, extensions)
				plugins := pluginFuncs()
				Expect(plugins).To(ContainElement(plugin1))
				Expect(plugins).To(ContainElement(plugin2))
			})

			It("should return plugins when funcs are used", func() {
				registryPlugins := func(opts bootstrap.Opts) []plugins.Plugin { return nil }
				extensions := Extensions{
					PluginExtensionsFuncs: []func() plugins.Plugin{
						func() plugins.Plugin { return plugin1 },
						func() plugins.Plugin { return plugin2 },
					},
				}

				pluginFuncs := GetPluginsWithExtensionsAndRegistry(opts, registryPlugins, extensions)
				plugins := pluginFuncs()
				Expect(plugins).To(HaveLen(2))
				Expect(plugins).To(ContainElement(plugin1))
				Expect(plugins).To(ContainElement(plugin2))
			})

			It("should return plugins and registry", func() {
				registryPlugins := func(opts bootstrap.Opts) []plugins.Plugin { return []plugins.Plugin{plugin1} }
				extensions := Extensions{
					PluginExtensions: []plugins.Plugin{
						plugin2,
					},
				}

				pluginFuncs := GetPluginsWithExtensionsAndRegistry(opts, registryPlugins, extensions)
				plugins := pluginFuncs()
				Expect(plugins).To(HaveLen(2))
				Expect(plugins).To(ContainElement(plugin1))
				Expect(plugins).To(ContainElement(plugin2))
			})

			It("should return plugin funcss and registry", func() {
				registryPlugins := func(opts bootstrap.Opts) []plugins.Plugin { return []plugins.Plugin{plugin1} }
				extensions := Extensions{
					PluginExtensionsFuncs: []func() plugins.Plugin{
						func() plugins.Plugin { return plugin2 },
					},
				}

				pluginFuncs := GetPluginsWithExtensionsAndRegistry(opts, registryPlugins, extensions)
				plugins := pluginFuncs()
				Expect(plugins).To(HaveLen(2))
				Expect(plugins).To(ContainElement(plugin1))
				Expect(plugins).To(ContainElement(plugin2))
			})
		})

		Context("Kube tests", func() {
			var (
				kubeCoreCache kube.SharedCache
			)
			BeforeEach(func() {
				if os.Getenv("RUN_KUBE_TESTS") != "1" {
					Skip("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
				}
				os.Setenv("AUTO_CREATE_CRDS", "1")
				settings.ConfigSource = &v1.Settings_KubernetesConfigSource{KubernetesConfigSource: &v1.Settings_KubernetesCrds{}}
				settings.SecretSource = &v1.Settings_KubernetesSecretSource{KubernetesSecretSource: &v1.Settings_KubernetesSecrets{}}
				settings.ArtifactSource = &v1.Settings_KubernetesArtifactSource{KubernetesArtifactSource: &v1.Settings_KubernetesConfigmaps{}}
				kubeCoreCache = kube.NewKubeCache(ctx)
			})

			AfterEach(func() {
				os.Unsetenv("AUTO_CREATE_CRDS")
			})

			It("can be called with core cache", func() {
				setup := NewSetupFunc()
				err := setup(ctx, kubeCoreCache, memcache, settings)
				Expect(err).NotTo(HaveOccurred())
			})

			It("can be called with core cache warming endpoints", func() {
				settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(time.Minute)
				setup := NewSetupFunc()
				err := setup(ctx, kubeCoreCache, memcache, settings)
				Expect(err).NotTo(HaveOccurred())
			})

			It("panics when endpoints don't arrive in a timely manner", func() {
				settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(1 * time.Nanosecond)
				setup := NewSetupFunc()
				Expect(func() { setup(ctx, kubeCoreCache, memcache, settings) }).To(Panic())
			})

			It("doesn't panic when endpoints don't arrive in a timely manner if set to zero", func() {
				settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(0)
				setup := NewSetupFunc()
				Expect(func() { setup(ctx, kubeCoreCache, memcache, settings) }).NotTo(Panic())
			})

		})
	})
})

func getRandomAddr() string {
	listener, err := net.Listen("tcp", "localhost:0")
	Expect(err).NotTo(HaveOccurred())
	addr := listener.Addr().String()
	listener.Close()
	return addr
}

type dummyPlugin struct{}

func (*dummyPlugin) Init(params plugins.InitParams) error { return nil }
