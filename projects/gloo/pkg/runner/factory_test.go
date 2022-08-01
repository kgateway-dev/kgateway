package runner_test

import (
	"context"
	"net"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/runner"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var _ = Describe("RunnerFactory", func() {

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

	AfterEach(func() {
		cancel()
	})

	Context("Setup", func() {

		BeforeEach(func() {
			settings = &v1.Settings{
				RefreshRate: prototime.DurationToProto(time.Hour),
				Gloo: &v1.GlooOptions{
					XdsBindAddr:        getRandomAddr(),
					ValidationBindAddr: getRandomAddr(),
				},
				DiscoveryNamespace: "non-existent-namespace",
				WatchNamespaces:    []string{"non-existent-namespace"},
				Gateway: &v1.GatewayOptions{
					EnableGatewayController: &wrapperspb.BoolValue{Value: true},
					PersistProxySpec:        &wrapperspb.BoolValue{Value: false},
					Validation:              nil,
				},
			}
			memcache = memory.NewInMemoryResourceCache()
			newContext()
		})

		Context("xds", func() {
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

			It("setup can be called twice", func() {
				runnerFactoryImpl := runner.NewRunnerFactory()

				runner, err := runnerFactoryImpl(ctx, nil, memcache, settings)
				Expect(err).NotTo(HaveOccurred())
				Expect(runner()).NotTo(HaveOccurred())

				testFunc := setupTestGrpcClient()

				newContext()
				runner, err = runnerFactoryImpl(ctx, nil, memcache, settings)
				Expect(err).NotTo(HaveOccurred())
				Expect(runner()).NotTo(HaveOccurred())

				// give things a chance to react
				time.Sleep(time.Second)

				// make sure that xds snapshot was not restarted
				err = testFunc()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("RunExtensions tests", func() {

			var (
				plugin1 = &dummyPlugin{}
				plugin2 = &dummyPlugin{}
			)

			It("should return plugins", func() {
				extensions := runner.RunExtensions{
					PluginRegistryFactory: func(ctx context.Context, opts runner.RunOpts) plugins.PluginRegistry {
						return registry.NewPluginRegistry([]plugins.Plugin{
							plugin1,
							plugin2,
						})
					},
				}

				pluginRegistry := extensions.PluginRegistryFactory(context.TODO(), runner.RunOpts{})
				plugins := pluginRegistry.GetPlugins()
				Expect(plugins).To(ContainElement(plugin1))
				Expect(plugins).To(ContainElement(plugin2))
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

func (*dummyPlugin) Name() string { return "dummy_plugin" }

func (*dummyPlugin) Init(_ plugins.InitParams) {
}
