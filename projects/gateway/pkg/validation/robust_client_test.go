package validation

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/rotisserie/eris"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var res = &validation.GlooValidationServiceResponse{}

type mockValidationService struct {
	err error
}

func (s *mockValidationService) Validate(context.Context, *validation.GlooValidationServiceRequest) (*validation.GlooValidationServiceResponse, error) {
	return res, s.err
}

func (s *mockValidationService) NotifyOnResync(*validation.NotifyOnResyncRequest, validation.GlooValidationService_NotifyOnResyncServer) error {
	panic("implement me")
}

func makeListener(errToReturn error, addr string) (string, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	grpcServer := grpc.NewServer()
	lis, err := net.Listen("tcp", addr)
	Expect(err).NotTo(HaveOccurred())

	validation.RegisterGlooValidationServiceServer(grpcServer, &mockValidationService{err: errToReturn})

	go func() {
		fmt.Println("starting")
		grpcServer.Serve(lis)
	}()
	go func() {
		<-ctx.Done()
		grpcServer.Stop()
		fmt.Println("shutting down")
	}()
	return lis.Addr().String(), cancel
}

var _ = Describe("RetryOnUnavailableClientConstructor", func() {

	It("creates a new client and closes the old one", func() {
		grpcAddr, cancel := makeListener(nil, "127.0.0.1:0")

		rootCtx := context.Background()
		constructor := RetryOnUnavailableClientConstructor(rootCtx, grpcAddr)

		client, err := constructor()
		Expect(err).NotTo(HaveOccurred())

		// sanity check
		resp, err := client.Validate(rootCtx, &validation.GlooValidationServiceRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(Equal(res))

		// shut down the server
		cancel()

		time.Sleep(100 * time.Millisecond) // wait for the server to shut down

		Eventually(func() codes.Code {
			_, err := client.Validate(rootCtx, &validation.GlooValidationServiceRequest{})
			return status.Code(err)
		}, "10s", "1s").Should(Equal(codes.Unavailable))

		// let the client connection retry backoff get long enough so
		time.Sleep(time.Millisecond * 10000)
		// recreate the listener on the same port
		_, cancel = makeListener(nil, grpcAddr)
		defer cancel()

		// conn should still be refused
		_, err = client.Validate(rootCtx, &validation.GlooValidationServiceRequest{})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.Unavailable))

		// new client should reestablish connection
		client, err = constructor()
		Expect(err).NotTo(HaveOccurred())

		resp, err = client.Validate(rootCtx, &validation.GlooValidationServiceRequest{})
		Expect(err).NotTo(HaveOccurred())

		Expect(resp).To(Equal(res))

	})

})

type mockWrappedValidationClient struct {
	name string
	err  error
}

func (c *mockWrappedValidationClient) NotifyOnResync(ctx context.Context, in *validation.NotifyOnResyncRequest, opts ...grpc.CallOption) (validation.GlooValidationService_NotifyOnResyncClient, error) {
	return nil, nil
}

func (c *mockWrappedValidationClient) Validate(ctx context.Context, in *validation.GlooValidationServiceRequest, opts ...grpc.CallOption) (*validation.GlooValidationServiceResponse, error) {
	return res, c.err
}

var _ = Describe("RobustClient", func() {

	It("swaps out the client when it returns a connection error", func() {
		original := &mockWrappedValidationClient{name: "original"}
		robustClient, _ := NewConnectionRefreshingValidationClient(func() (client validation.GlooValidationServiceClient, e error) {
			return original, nil
		})

		rootCtx := context.Background()

		resp, err := robustClient.Validate(rootCtx, &validation.GlooValidationServiceRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(Equal(res))

		// make the original client return an error
		original.err = status.Error(codes.Unavailable, "oh no, an error")
		// update the constructor func with a new client
		replacement := &mockWrappedValidationClient{name: "replacement"}
		robustClient.constructValidationClient = func() (client validation.GlooValidationServiceClient, e error) {
			return replacement, nil
		}

		// robust client should replace with the working client
		resp, err = robustClient.Validate(rootCtx, &validation.GlooValidationServiceRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(Equal(res))

		robustClient.lock.RLock()
		Expect(robustClient.validationClient).To(Equal(replacement))
		robustClient.lock.RUnlock()
	})

	It("does not swap out the client when new client returns a connection error", func() {
		original := &mockWrappedValidationClient{name: "original"}
		robustClient, _ := NewConnectionRefreshingValidationClient(func() (client validation.GlooValidationServiceClient, e error) {
			return original, nil
		})

		rootCtx := context.Background()

		resp, err := robustClient.Validate(rootCtx, &validation.GlooValidationServiceRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(Equal(res))

		// make the original client return an error
		original.err = status.Error(codes.Unavailable, "oh no, an error")
		// update the constructor func with a new client, that also returns an error
		robustClient.constructValidationClient = func() (client validation.GlooValidationServiceClient, e error) {
			return nil, eris.New("intentionally failed to connect")
		}

		// robust client should not replace with the replacement client since it could not be constructed
		resp, err = robustClient.Validate(rootCtx, &validation.GlooValidationServiceRequest{})
		Expect(err).To(HaveOccurred())
		Expect(err).To(ContainElement(original.err))

		robustClient.lock.RLock()
		Expect(robustClient.validationClient).To(Equal(original))
		robustClient.lock.RUnlock()
	})
})
