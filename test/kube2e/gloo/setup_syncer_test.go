package gloo_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/runner"

	"github.com/solo-io/gloo/pkg/utils/runutils"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v1alpha1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	k2e "github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"github.com/solo-io/solo-kit/test/helpers"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/rest"
)

var _ = Describe("SetupSyncer", func() {

	var (
		settings  *v1.Settings
		ctx       context.Context
		cancel    context.CancelFunc
		memcache  memory.InMemoryResourceCache
		setupLock sync.RWMutex
	)

	newContext := func() {
		if cancel != nil {
			cancel()
		}
		ctx, cancel = context.WithCancel(context.Background())
		ctx = settingsutil.WithSettings(ctx, settings)
	}

	// RunnerFactory is used to configure Gloo with appropriate configuration
	// It is assumed to run once at construction time, and therefore it executes directives that
	// are also assumed to only run at construction time.
	// One of those, is the construction of schemes: https://github.com/kubernetes/kubernetes/pull/89019#issuecomment-600278461
	// In our tests we do not follow this pattern, and to avoid data races (that cause test failures)
	// we ensure that only 1 RunnerFactory is ever called at a time
	newSynchronizedSetupFunc := func() runutils.SetupFunc {
		setupFunc := runner.NewRunnerFactory()

		var synchronizedSetupFunc runutils.SetupFunc
		synchronizedSetupFunc = func(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *v1.Settings) error {
			setupLock.Lock()
			defer setupLock.Unlock()
			return setupFunc(ctx, kubeCache, inMemoryCache, settings)
		}

		return synchronizedSetupFunc
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

				setup := newSynchronizedSetupFunc()

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

		Context("Kube tests", func() {

			var (
				kubeCoreCache    kube.SharedCache
				registerCrdsOnce sync.Once
				cfg              *rest.Config
			)

			registerCRDs := func() {
				var err error
				cfg, err = kubeutils.GetConfig("", "")
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				cs, err := clientset.NewForConfig(cfg)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				crdsToRegister := []crd.Crd{
					v1.UpstreamCrd,
					v1.UpstreamGroupCrd,
					v1.ProxyCrd,
					v1.SettingsCrd,
					gatewayv1.GatewayCrd,
					extauthv1.AuthConfigCrd,
					v1alpha1.RateLimitConfigCrd,
					v1beta1.GraphQLApiCrd,
					gatewayv1.VirtualServiceCrd,
					gatewayv1.RouteOptionCrd,
					gatewayv1.VirtualHostOptionCrd,
					gatewayv1.RouteTableCrd,
					gatewayv1.MatchableHttpGatewayCrd,
				}

				for _, crdToRegister := range crdsToRegister {
					err = helpers.AddAndRegisterCrd(ctx, crdToRegister, cs)
					ExpectWithOffset(1, err).NotTo(HaveOccurred())
				}
			}

			BeforeEach(func() {
				if os.Getenv("RUN_KUBE_TESTS") != "1" {
					Skip("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
				}
				settings = &v1.Settings{
					RefreshRate: prototime.DurationToProto(time.Hour),
					Gloo: &v1.GlooOptions{
						XdsBindAddr:        getRandomAddr(),
						ValidationBindAddr: getRandomAddr(),
					},
					ConfigSource:       &v1.Settings_KubernetesConfigSource{KubernetesConfigSource: &v1.Settings_KubernetesCrds{}},
					SecretSource:       &v1.Settings_KubernetesSecretSource{KubernetesSecretSource: &v1.Settings_KubernetesSecrets{}},
					ArtifactSource:     &v1.Settings_KubernetesArtifactSource{KubernetesArtifactSource: &v1.Settings_KubernetesConfigmaps{}},
					DiscoveryNamespace: "non-existent-namespace",
					WatchNamespaces:    []string{"non-existent-namespace"},
					Gateway: &v1.GatewayOptions{
						EnableGatewayController: &wrapperspb.BoolValue{Value: true},
						PersistProxySpec:        &wrapperspb.BoolValue{Value: false},
						Validation: &v1.GatewayOptions_ValidationOptions{
							ValidationServerGrpcMaxSizeBytes: &wrappers.Int32Value{Value: 4000000},
						},
					},
				}
				kubeCoreCache = kube.NewKubeCache(ctx)

				// Gloo RunnerFactory is no longer responsible for registering CRDs. This was not used in production, and
				// required Gloo having RBAC permissions that it should not have. CRD registration is now only supported
				// by Helm. Therefore, this test needs to manually register CRDs to test setup.
				registerCrdsOnce.Do(registerCRDs)
			})

			It("can be called with core cache", func() {
				setup := newSynchronizedSetupFunc()
				err := setup(ctx, kubeCoreCache, memcache, settings)
				Expect(err).NotTo(HaveOccurred())
			})

			It("can be called with core cache warming endpoints", func() {
				settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(time.Minute)
				setup := newSynchronizedSetupFunc()
				err := setup(ctx, kubeCoreCache, memcache, settings)
				Expect(err).NotTo(HaveOccurred())
			})

			It("panics when endpoints don't arrive in a timely manner", func() {
				settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(1 * time.Nanosecond)
				setup := newSynchronizedSetupFunc()
				Expect(func() { setup(ctx, kubeCoreCache, memcache, settings) }).To(Panic())
			})

			It("doesn't panic when endpoints don't arrive in a timely manner if set to zero", func() {
				settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(0)
				setup := newSynchronizedSetupFunc()
				Expect(func() { setup(ctx, kubeCoreCache, memcache, settings) }).NotTo(Panic())
			})

			setupTestGrpcClient := func() func() error {
				var cc *grpc.ClientConn
				var err error
				Eventually(func() error {
					cc, err = grpc.DialContext(ctx, "localhost:9988", grpc.WithInsecure(), grpc.WithBlock(), grpc.FailOnNonTempDialError(true))
					return err
				}, "10s", "1s").Should(BeNil())
				// setup a gRPC client to make sure connection is persistent across invocations
				client := validation.NewGlooValidationServiceClient(cc)
				req := &validation.GlooValidationServiceRequest{Proxy: &v1.Proxy{Listeners: []*v1.Listener{{Name: "test-listener"}}}}
				return func() error {
					_, err := client.Validate(ctx, req)
					return err
				}
			}

			startPortFwd := func() *os.Process {
				validationPort := strconv.Itoa(9988)
				portFwd := exec.Command("kubectl", "port-forward", "-n", namespace,
					"deployment/gloo", validationPort)
				portFwd.Stdout = os.Stderr
				portFwd.Stderr = os.Stderr
				err := portFwd.Start()
				Expect(err).ToNot(HaveOccurred())
				return portFwd.Process
			}

			It("restarts validation grpc server when settings change", func() {
				// setup port forward
				portFwdProc := startPortFwd()
				defer func() {
					if portFwdProc != nil {
						portFwdProc.Kill()
					}
				}()

				testFunc := setupTestGrpcClient()
				err := testFunc()
				Expect(err).NotTo(HaveOccurred())

				k2e.UpdateSettings(ctx, func(settings *v1.Settings) {
					settings.Gateway.Validation.ValidationServerGrpcMaxSizeBytes = &wrappers.Int32Value{Value: 1}
				}, namespace)

				err = testFunc()
				Expect(err.Error()).To(ContainSubstring("received message larger than max (19 vs. 1)"))
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
