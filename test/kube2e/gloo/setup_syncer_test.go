package gloo_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/projects/gloo/pkg/runner"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	k2e "github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var _ = Describe("SetupSyncer", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc

		setupLock sync.RWMutex
	)

	// Runner is used to configure Gloo with appropriate configuration
	// It is assumed to run once at construction time, and therefore it executes directives that
	// are also assumed to only run at construction time.
	// One of those, is the construction of schemes: https://github.com/kubernetes/kubernetes/pull/89019#issuecomment-600278461
	// In our tests we do not follow this pattern, and to avoid data races (that cause test failures)
	// we ensure that only 1 Runner is ever called at a time
	newSynchronizedRunner := func() bootstrap.Runner {
		runner := &SynchronizedRunner{
			Runner:  runner.NewGlooRunner(),
			RunLock: setupLock,
		}

		return runner
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	Context("Setup Kube Tests", func() {

		var (
			kubeCoreCache kube.SharedCache
			memCache      memory.InMemoryResourceCache

			settings *v1.Settings
		)

		BeforeEach(func() {
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
			memCache = memory.NewInMemoryResourceCache()
		})

		It("can be called with core cache", func() {
			runner := newSynchronizedRunner()
			err := runner.Run(ctx, kubeCoreCache, memCache, settings)
			Expect(err).NotTo(HaveOccurred())
		})

		It("can be called with core cache warming endpoints", func() {
			settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(time.Minute)
			runner := newSynchronizedRunner()
			err := runner.Run(ctx, kubeCoreCache, memCache, settings)
			Expect(err).NotTo(HaveOccurred())
		})

		It("panics when endpoints don't arrive in a timely manner", func() {
			settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(1 * time.Nanosecond)
			runner := newSynchronizedRunner()
			Expect(func() { runner.Run(ctx, kubeCoreCache, memCache, settings) }).To(Panic())
		})

		It("doesn't panic when endpoints don't arrive in a timely manner if set to zero", func() {
			settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(0)
			runner := newSynchronizedRunner()
			Expect(func() { runner.Run(ctx, kubeCoreCache, memCache, settings) }).NotTo(Panic())
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

func getRandomAddr() string {
	listener, err := net.Listen("tcp", "localhost:0")
	Expect(err).NotTo(HaveOccurred())
	addr := listener.Addr().String()
	listener.Close()
	return addr
}

var _ bootstrap.Runner = new(SynchronizedRunner)

// Runner.Run is used to configure Gloo with appropriate configuration
// It is assumed to run once at construction time, and therefore it executes directives that
// are also assumed to only run at construction time.
// One of those, is the construction of schemes: https://github.com/kubernetes/kubernetes/pull/89019#issuecomment-600278461
// In our tests we do not follow this pattern, and to avoid data races (that cause test failures)
// we ensure that only 1 Runner is ever called at a time
type SynchronizedRunner struct {
	Runner  bootstrap.Runner
	RunLock sync.RWMutex
}

func (s *SynchronizedRunner) Run(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *v1.Settings) error {
	s.RunLock.Lock()
	defer s.RunLock.Unlock()
	return s.Runner.Run(ctx, kubeCache, inMemoryCache, settings)
}
