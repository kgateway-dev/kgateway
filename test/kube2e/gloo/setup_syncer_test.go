package gloo_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector/singlereplica"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"google.golang.org/grpc"
)

var _ = Describe("Setup Syncer", func() {

	var (
		setupLock sync.RWMutex
	)

	// SetupFunc is used to configure Gloo with appropriate configuration
	// It is assumed to run once at construction time, and therefore it executes directives that
	// are also assumed to only run at construction time.
	// One of those, is the construction of schemes: https://github.com/kubernetes/kubernetes/pull/89019#issuecomment-600278461
	// In our tests we do not follow this pattern, and to avoid data races (that cause test failures)
	// we ensure that only 1 SetupFunc is ever called at a time
	newSynchronizedSetupFunc := func() setuputils.SetupFunc {
		setupFunc := setup.NewSetupFunc()

		var synchronizedSetupFunc setuputils.SetupFunc
		synchronizedSetupFunc = func(setupCtx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *v1.Settings, identity leaderelector.Identity) error {
			setupLock.Lock()
			defer setupLock.Unlock()
			// This is normally performed within the SetupSyncer and is required by Gloo components
			setupCtx = settingsutil.WithSettings(setupCtx, settings)

			return setupFunc(setupCtx, kubeCache, inMemoryCache, settings, identity)
		}

		return synchronizedSetupFunc
	}

	Context("Kube Tests", func() {

		var (
			kubeCoreCache kube.SharedCache
			memCache      memory.InMemoryResourceCache

			settings *v1.Settings
		)

		BeforeEach(func() {
			var err error
			settings, err = resourceClientset.SettingsClient().Read(testHelper.InstallNamespace, defaults.SettingsName, clients.ReadOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			settings.Gateway.Validation = nil
			settings.Gloo = &v1.GlooOptions{
				XdsBindAddr:        getRandomAddr(),
				ValidationBindAddr: getRandomAddr(),
				ProxyDebugBindAddr: getRandomAddr(),
			}

			kubeCoreCache = kube.NewKubeCache(ctx)
			memCache = memory.NewInMemoryResourceCache()
		})

		It("can be called with core cache", func() {
			setup := newSynchronizedSetupFunc()
			err := setup(ctx, kubeCoreCache, memCache, settings, singlereplica.Identity())
			Expect(err).NotTo(HaveOccurred())
		})

		It("can be called with core cache warming endpoints", func() {
			settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(time.Minute)
			setup := newSynchronizedSetupFunc()
			err := setup(ctx, kubeCoreCache, memCache, settings, singlereplica.Identity())
			Expect(err).NotTo(HaveOccurred())
		})

		It("panics when endpoints don't arrive in a timely manner", func() {
			settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(1 * time.Nanosecond)
			setup := newSynchronizedSetupFunc()
			Expect(func() {
				_ = setup(ctx, kubeCoreCache, memCache, settings, singlereplica.Identity())
			}).To(Panic())
		})

		It("doesn't panic when endpoints don't arrive in a timely manner if set to zero", func() {
			settings.Gloo.EndpointsWarmingTimeout = prototime.DurationToProto(0)
			setup := newSynchronizedSetupFunc()
			Expect(func() {
				_ = setup(ctx, kubeCoreCache, memCache, settings, singlereplica.Identity())
			}).NotTo(Panic())
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
			validationPort := strconv.Itoa(defaults.GlooValidationPort)
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

			kube2e.UpdateSettings(ctx, func(settings *v1.Settings) {
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
	err = listener.Close()
	Expect(err).NotTo(HaveOccurred())
	return addr
}
