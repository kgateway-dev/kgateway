package syncer_test

import (
	"context"
	"net"
	"time"

	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

var _ = Describe("SetupSyncer", func() {

	var (
		settings *v1.Settings
		ctx      context.Context
		cancel   context.CancelFunc
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
			RefreshRate: types.DurationProto(time.Hour),
			Gloo: &v1.GlooOptions{
				XdsBindAddr:        getRandomAddr(),
				ValidationBindAddr: getRandomAddr(),
			},
			DiscoveryNamespace: "non-existent-namespace",
			WatchNamespaces:    []string{"non-existent-namespace"},
		}
		newContext()
	})
	AfterEach(func() {
		cancel()
	})

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
		setup := NewSetupFunc()

		err := setup(ctx, nil, memory.NewInMemoryResourceCache(), settings)
		Expect(err).NotTo(HaveOccurred())

		testFunc := setupTestGrpcClient()

		newContext()
		err = setup(ctx, nil, memory.NewInMemoryResourceCache(), settings)
		Expect(err).NotTo(HaveOccurred())

		// give things a chance to react
		time.Sleep(time.Second)

		// make sure that xds snapshot was not restarted
		err = testFunc()
		Expect(err).NotTo(HaveOccurred())
	})

})

func getRandomAddr() string {
	listener, err := net.Listen("tcp", "localhost:0")
	Expect(err).NotTo(HaveOccurred())
	addr := listener.Addr().String()
	listener.Close()
	return addr
}
